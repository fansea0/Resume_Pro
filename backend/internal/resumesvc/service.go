package resumesvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/files"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/targetjob"
)

type Service struct {
	repo      *resume.Repository
	storage   files.FileStorage
	targetSvc *targetjob.Service
}

var ErrAvatarRetention = errors.New("avatar retention failed")

func NewService(repo *resume.Repository, storage files.FileStorage, targetSvc *targetjob.Service) *Service {
	return &Service{repo: repo, storage: storage, targetSvc: targetSvc}
}

type CreateInput struct {
	Principal  auth.Principal
	Title      string
	TemplateID string
	Content    json.RawMessage
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*resume.Resume, error) {
	targetJobID, err := s.validateTargetJobID(ctx, in.Principal, in.Content)
	if err != nil {
		return nil, err
	}

	key := avatarStorageKey(in.Content)
	if s.storage != nil && key != "" {
		if err := s.storage.Retain(ctx, avatarOwner(in.Principal), key); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrAvatarRetention, err)
		}
	}

	item, err := s.repo.Create(ctx, resume.CreateInput{
		TenantID:    in.Principal.TenantID,
		TargetJobID: targetJobID,
		AvatarOwner: avatarOwner(in.Principal),
		Title:       in.Title,
		TemplateID:  in.TemplateID,
		Content:     string(in.Content),
	})
	if err != nil {
		if s.storage != nil && key != "" && !isAvatarConflict(err) {
			_ = s.storage.Release(ctx, avatarOwner(in.Principal), key)
		}
		return nil, targetJobError(targetJobID, err)
	}
	return &item, nil
}

type UpdateInput struct {
	Principal  auth.Principal
	Title      string
	TemplateID string
	Content    json.RawMessage
}

func (s *Service) Update(ctx context.Context, explicitTenantID, id int64, in UpdateInput) (*resume.Resume, error) {
	if explicitTenantID != in.Principal.TenantID {
		return nil, resume.ErrNotFound
	}
	tenantID := in.Principal.TenantID
	targetJobID, err := s.validateTargetJobID(ctx, in.Principal, in.Content)
	if err != nil {
		return nil, err
	}

	previous, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	oldKey := avatarStorageKey(json.RawMessage(previous.Content))
	newKey := avatarStorageKey(in.Content)
	retainedNew := s.storage != nil && newKey != "" && newKey != oldKey
	if retainedNew {
		if err := s.storage.Retain(ctx, avatarOwner(in.Principal), newKey); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrAvatarRetention, err)
		}
	}

	item, err := s.repo.Update(ctx, tenantID, id, resume.UpdateInput{
		TargetJobID: targetJobID,
		AvatarOwner: avatarOwner(in.Principal),
		Title:       in.Title,
		TemplateID:  in.TemplateID,
		Content:     string(in.Content),
	})
	if err != nil {
		if retainedNew && !isAvatarConflict(err) {
			_ = s.storage.Release(ctx, avatarOwner(in.Principal), newKey)
		}
		return nil, targetJobError(targetJobID, err)
	}
	if s.storage != nil && oldKey != "" && oldKey != newKey {
		_ = s.storage.Release(ctx, avatarOwner(in.Principal), oldKey)
	}
	return &item, nil
}

func (s *Service) Delete(ctx context.Context, principal auth.Principal, id int64) error {
	previous, err := s.repo.Get(ctx, principal.TenantID, id)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, principal.TenantID, id); err != nil {
		return err
	}
	if key := avatarStorageKey(json.RawMessage(previous.Content)); s.storage != nil && key != "" {
		_ = s.storage.Release(ctx, avatarOwner(principal), key)
	}
	return nil
}

func targetJobID(content json.RawMessage) (*int64, error) {
	var document struct {
		TargetJobID *float64 `json:"targetJobId"`
	}
	if err := json.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("invalid content: %w", err)
	}
	if document.TargetJobID == nil {
		return nil, nil
	}
	id := int64(*document.TargetJobID)
	return &id, nil
}

func (s *Service) validateTargetJobID(ctx context.Context, principal auth.Principal, content json.RawMessage) (*int64, error) {
	id, err := targetJobID(content)
	if err != nil || id == nil {
		return id, err
	}
	if s.targetSvc == nil {
		return nil, fmt.Errorf("target job %d cannot be verified", *id)
	}
	if _, err := s.targetSvc.Get(ctx, principal.TenantID, *id); err != nil {
		if errors.Is(err, targetjob.ErrNotFound) {
			return nil, fmt.Errorf("target job %d not found or not owned by tenant", *id)
		}
		return nil, err
	}
	return id, nil
}

func targetJobError(targetJobID *int64, err error) error {
	if targetJobID != nil && errors.Is(err, resume.ErrTargetJobNotFound) {
		return fmt.Errorf("target job %d not found or not owned by tenant", *targetJobID)
	}
	return err
}

func isAvatarConflict(err error) bool {
	return errors.Is(err, resume.ErrAvatarKeyInUse) || errors.Is(err, resume.ErrAvatarKeyReserved)
}

func avatarOwner(principal auth.Principal) string {
	return fmt.Sprintf("user-%d", principal.UserID)
}

func avatarStorageKey(content json.RawMessage) string {
	var document struct {
		Profile struct {
			AvatarStorageKey string `json:"avatarStorageKey"`
		} `json:"profile"`
	}
	if json.Unmarshal(content, &document) != nil {
		return ""
	}
	return document.Profile.AvatarStorageKey
}

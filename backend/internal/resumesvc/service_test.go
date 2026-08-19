package resumesvc_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/files"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/resumesvc"
	"resumepro/backend/internal/store"
	"resumepro/backend/internal/targetjob"
)

func TestServiceCreateValidatesTargetJobOwnership(t *testing.T) {
	svc, _, targetSvc, _ := setupService(t, nil)
	ctx := context.Background()
	target, err := targetSvc.Create(ctx, 1, targetjob.CreateInput{Requirements: "Build reliable backend services"})
	require.NoError(t, err)

	created, err := svc.Create(ctx, createInput(principal(1, 1), resumeContent(target.ID, "")))
	require.NoError(t, err)
	assert.Equal(t, int64(1), created.TenantID)

	_, err = svc.Create(ctx, createInput(principal(2, 2), resumeContent(target.ID, "")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target job")
}

func TestServiceCreateAcceptsNoTargetJobID(t *testing.T) {
	svc, _, _, _ := setupService(t, nil)

	created, err := svc.Create(context.Background(), createInput(principal(1, 1), resumeContent(0, "")))

	require.NoError(t, err)
	assert.NotZero(t, created.ID)
}

func TestServiceUpdateValidatesTargetJobOwnershipAndScopesResumeByTenant(t *testing.T) {
	svc, repo, targetSvc, _ := setupService(t, nil)
	ctx := context.Background()
	created, err := svc.Create(ctx, createInput(principal(1, 1), resumeContent(0, "")))
	require.NoError(t, err)
	foreignTarget, err := targetSvc.Create(ctx, 2, targetjob.CreateInput{Requirements: "Own the frontend delivery roadmap"})
	require.NoError(t, err)

	_, err = svc.Update(ctx, 1, created.ID, resumesvc.UpdateInput{
		Principal: principal(1, 1), Title: "Changed", TemplateID: "business", Content: resumeContent(foreignTarget.ID, ""),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target job")
	unchanged, err := repo.Get(ctx, 1, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "Test resume", unchanged.Title)

	_, err = svc.Update(ctx, 2, created.ID, resumesvc.UpdateInput{
		Principal: principal(2, 2), Title: "Stolen", TemplateID: "business", Content: resumeContent(0, ""),
	})
	require.ErrorIs(t, err, resume.ErrNotFound)
	unchanged, err = repo.Get(ctx, 1, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "Test resume", unchanged.Title)
}

func TestServiceUpdateRejectsMismatchedExplicitAndPrincipalTenants(t *testing.T) {
	svc, repo, targetSvc, _ := setupService(t, nil)
	ctx := context.Background()
	created, err := svc.Create(ctx, createInput(principal(1, 1), resumeContent(0, "")))
	require.NoError(t, err)
	tenantTwoTarget, err := targetSvc.Create(ctx, 2, targetjob.CreateInput{Requirements: "Lead tenant two platform delivery"})
	require.NoError(t, err)

	_, err = svc.Update(ctx, 1, created.ID, resumesvc.UpdateInput{
		Principal:  principal(2, 2),
		Title:      "Cross-tenant rewrite",
		TemplateID: "business",
		Content:    resumeContent(tenantTwoTarget.ID, ""),
	})

	require.ErrorIs(t, err, resume.ErrNotFound)
	unchanged, getErr := repo.Get(ctx, 1, created.ID)
	require.NoError(t, getErr)
	assert.Equal(t, "Test resume", unchanged.Title)
	assert.JSONEq(t, string(resumeContent(0, "")), unchanged.Content)
}

func TestServicePreservesAvatarLifecycle(t *testing.T) {
	storage := newRecordingStorage()
	svc, _, _, _ := setupService(t, storage)
	ctx := context.Background()
	p := principal(7, 1)
	firstKey := "550e8400-e29b-41d4-a716-446655440000"
	secondKey := "550e8400-e29b-41d4-a716-446655440001"

	created, err := svc.Create(ctx, createInput(p, resumeContent(0, firstKey)))
	require.NoError(t, err)
	assert.Equal(t, []storageCall{{owner: "user-7", key: firstKey}}, storage.retained)

	updated, err := svc.Update(ctx, 1, created.ID, resumesvc.UpdateInput{
		Principal: p, Title: "Updated resume", TemplateID: "business", Content: resumeContent(0, secondKey),
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated resume", updated.Title)
	assert.Equal(t, []storageCall{{owner: "user-7", key: firstKey}, {owner: "user-7", key: secondKey}}, storage.retained)
	assert.Equal(t, []storageCall{{owner: "user-7", key: firstKey}}, storage.released)

	require.NoError(t, svc.Delete(ctx, p, created.ID))
	assert.Equal(t, []storageCall{{owner: "user-7", key: firstKey}, {owner: "user-7", key: secondKey}}, storage.released)
}

func TestServiceCreateDoesNotPersistWhenAvatarRetentionFails(t *testing.T) {
	storage := newRecordingStorage()
	storage.retainErr = errors.New("storage unavailable")
	svc, repo, _, _ := setupService(t, storage)

	_, err := svc.Create(context.Background(), createInput(principal(1, 1), resumeContent(0, "550e8400-e29b-41d4-a716-446655440000")))

	require.ErrorContains(t, err, "storage unavailable")
	items, listErr := repo.List(context.Background(), 1)
	require.NoError(t, listErr)
	assert.Empty(t, items)
}

func TestServiceDoesNotReleaseAvatarOnKeyConflict(t *testing.T) {
	storage := newRecordingStorage()
	svc, _, _, _ := setupService(t, storage)
	ctx := context.Background()
	key := "550e8400-e29b-41d4-a716-446655440000"
	input := createInput(principal(1, 1), resumeContent(0, key))
	_, err := svc.Create(ctx, input)
	require.NoError(t, err)

	_, err = svc.Create(ctx, input)

	require.ErrorIs(t, err, resume.ErrAvatarKeyInUse)
	assert.Empty(t, storage.released, "a conflicting save must not delete an avatar already bound to a resume")
}

func TestServiceReleasesNewAvatarWhenUpdateFails(t *testing.T) {
	storage := newRecordingStorage()
	svc, _, _, db := setupService(t, storage)
	ctx := context.Background()
	p := principal(1, 1)
	oldKey := "550e8400-e29b-41d4-a716-446655440000"
	newKey := "550e8400-e29b-41d4-a716-446655440001"
	created, err := svc.Create(ctx, createInput(p, resumeContent(0, oldKey)))
	require.NoError(t, err)

	// Force a repository error after Retain succeeds, proving the service rolls
	// back the newly retained file while leaving the previous avatar untouched.
	_, err = db.Exec(`CREATE TRIGGER fail_resume_update BEFORE UPDATE ON resumes BEGIN SELECT RAISE(ABORT, 'forced update failure'); END`)
	require.NoError(t, err)
	_, err = svc.Update(ctx, 1, created.ID, resumesvc.UpdateInput{
		Principal: p, Title: "Updated", TemplateID: "business", Content: resumeContent(0, newKey),
	})
	require.ErrorContains(t, err, "forced update failure")
	assert.Equal(t, []storageCall{{owner: "user-1", key: newKey}}, storage.released)
}

func setupService(t *testing.T, storage files.FileStorage) (*resumesvc.Service, *resume.Repository, *targetjob.Service, *sqlx.DB) {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, store.Migrate(context.Background(), db))
	resumeRepo := resume.NewRepository(db)
	targetSvc := targetjob.NewService(targetjob.NewRepository(db))
	svc := resumesvc.NewService(resumeRepo, storage, targetSvc)
	return svc, resumeRepo, targetSvc, db
}

func principal(userID, tenantID int64) auth.Principal {
	return auth.Principal{UserID: userID, TenantID: tenantID}
}

func createInput(p auth.Principal, content json.RawMessage) resumesvc.CreateInput {
	return resumesvc.CreateInput{Principal: p, Title: "Test resume", TemplateID: "minimal", Content: content}
}

func resumeContent(targetJobID int64, avatarKey string) json.RawMessage {
	target := ""
	if targetJobID != 0 {
		target = fmt.Sprintf(`,"targetJobId":%d`, targetJobID)
	}
	return json.RawMessage(fmt.Sprintf(`{
		"schemaVersion":2,
		"title":"Test resume",
		"templateId":"minimal",
		"profile":{"fullName":"Ada","headline":"Engineer","email":"ada@example.com","phone":"","location":"","website":"","avatarUrl":"","avatarStorageKey":%q},
		"sections":[]%s
	}`, avatarKey, target))
}

type storageCall struct {
	owner string
	key   string
}

type recordingStorage struct {
	retained  []storageCall
	released  []storageCall
	retainErr error
}

func newRecordingStorage() *recordingStorage { return &recordingStorage{} }

func (s *recordingStorage) Store(context.Context, string, string, files.File) (files.StoredFile, error) {
	return files.StoredFile{}, nil
}

func (s *recordingStorage) PublicURL(string, string) string { return "" }

func (s *recordingStorage) Retain(_ context.Context, owner, key string) error {
	if s.retainErr != nil {
		return s.retainErr
	}
	s.retained = append(s.retained, storageCall{owner: owner, key: key})
	return nil
}

func (s *recordingStorage) Release(_ context.Context, owner, key string) error {
	s.released = append(s.released, storageCall{owner: owner, key: key})
	return nil
}

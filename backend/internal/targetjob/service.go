package targetjob

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

const (
	minRequirementsLen = 10
	maxFieldLen        = 200
	maxRequirementsLen = 20000
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

// NewServiceFromDB constructs a service directly from a database connection.
func NewServiceFromDB(db *sqlx.DB) *Service { return NewService(NewRepository(db)) }

func validate(in CreateInput) error {
	requirements := strings.TrimSpace(in.Requirements)
	if len(requirements) < minRequirementsLen {
		return fmt.Errorf("%w: requirements must be at least %d characters after trim", ErrValidation, minRequirementsLen)
	}
	if len(in.Company) > maxFieldLen || len(in.Position) > maxFieldLen {
		return fmt.Errorf("%w: company/position must be at most %d characters", ErrValidation, maxFieldLen)
	}
	if len(requirements) > maxRequirementsLen {
		return fmt.Errorf("%w: requirements must be at most %d characters", ErrValidation, maxRequirementsLen)
	}
	return nil
}

func (s *Service) Create(ctx context.Context, tenantID int64, in CreateInput) (*TargetJob, error) {
	in.Requirements = strings.TrimSpace(in.Requirements)
	if err := validate(in); err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, tenantID, in)
}

func (s *Service) Get(ctx context.Context, tenantID, id int64) (*TargetJob, error) {
	return s.repo.Get(ctx, tenantID, id)
}

func (s *Service) List(ctx context.Context, tenantID int64) ([]TargetJob, error) {
	return s.repo.List(ctx, tenantID)
}

func (s *Service) Update(ctx context.Context, tenantID, id int64, in UpdateInput) (*TargetJob, error) {
	in.Requirements = strings.TrimSpace(in.Requirements)
	if err := validate(in); err != nil {
		return nil, err
	}
	return s.repo.Update(ctx, tenantID, id, in)
}

func (s *Service) Delete(ctx context.Context, tenantID, id int64) (int64, error) {
	if _, err := s.repo.Get(ctx, tenantID, id); err != nil {
		return 0, err
	}

	count, err := s.repo.CountByTargetJob(ctx, tenantID, id)
	if err != nil {
		return 0, err
	}
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return 0, err
	}
	return count, nil
}

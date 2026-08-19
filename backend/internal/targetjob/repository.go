package targetjob

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

type Repository struct{ db *sqlx.DB }

func NewRepository(db *sqlx.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, tenantID int64, in CreateInput) (*TargetJob, error) {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO target_jobs (tenant_id, company, position, requirements, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		tenantID, in.Company, in.Position, in.Requirements, now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, tenantID, id)
}

func (r *Repository) Get(ctx context.Context, tenantID, id int64) (*TargetJob, error) {
	var tj TargetJob
	err := r.db.GetContext(ctx, &tj, `
		SELECT id, tenant_id, company, position, requirements, created_at, updated_at
		FROM target_jobs WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tj, nil
}

func (r *Repository) List(ctx context.Context, tenantID int64) ([]TargetJob, error) {
	var list []TargetJob
	err := r.db.SelectContext(ctx, &list, `
		SELECT id, tenant_id, company, position, requirements, created_at, updated_at
		FROM target_jobs WHERE tenant_id = ? ORDER BY updated_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (r *Repository) Update(ctx context.Context, tenantID, id int64, in UpdateInput) (*TargetJob, error) {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx, `
		UPDATE target_jobs
		SET company = ?, position = ?, requirements = ?, updated_at = ?
		WHERE id = ? AND tenant_id = ?`,
		in.Company, in.Position, in.Requirements, now, id, tenantID)
	if err != nil {
		return nil, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, ErrNotFound
	}
	return r.Get(ctx, tenantID, id)
}

func (r *Repository) Delete(ctx context.Context, tenantID, id int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM target_jobs WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) CountByTargetJob(ctx context.Context, tenantID, id int64) (int64, error) {
	var n int64
	err := r.db.GetContext(ctx, &n, `
		SELECT COUNT(*) FROM resumes
		WHERE tenant_id = ?
		  AND json_extract(content, '$.targetJobId') = ?`,
		tenantID, id)
	return n, err
}

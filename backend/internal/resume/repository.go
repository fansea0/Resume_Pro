package resume

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

var ErrNotFound = errors.New("resume not found")

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (Resume, error) {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `INSERT INTO resumes (tenant_id, title, template_id, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, input.TenantID, input.Title, input.TemplateID, input.Content, now, now)
	if err != nil {
		return Resume{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Resume{}, err
	}
	return r.Get(ctx, input.TenantID, id)
}

func (r *Repository) Get(ctx context.Context, tenantID, id int64) (Resume, error) {
	var item Resume
	err := r.db.GetContext(ctx, &item, `SELECT id, tenant_id, title, template_id, content, created_at, updated_at FROM resumes WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return Resume{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) List(ctx context.Context, tenantID int64) ([]Resume, error) {
	items := []Resume{}
	err := r.db.SelectContext(ctx, &items, `SELECT id, tenant_id, title, template_id, content, created_at, updated_at FROM resumes WHERE tenant_id = ? ORDER BY updated_at DESC, id DESC`, tenantID)
	return items, err
}

// AvatarStorageKeyInUse reports whether another current resume in the tenant
// owns key. Content is parsed instead of searched as text so formatting changes
// cannot create a false match.
func (r *Repository) AvatarStorageKeyInUse(ctx context.Context, tenantID, excludeID int64, key string) (bool, error) {
	items, err := r.List(ctx, tenantID)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.ID == excludeID {
			continue
		}
		var content struct {
			Profile struct {
				AvatarStorageKey string `json:"avatarStorageKey"`
			} `json:"profile"`
		}
		if json.Unmarshal([]byte(item.Content), &content) == nil && content.Profile.AvatarStorageKey == key {
			return true, nil
		}
	}
	return false, nil
}

func (r *Repository) Update(ctx context.Context, tenantID, id int64, input UpdateInput) (Resume, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return Resume{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var current Resume
	err = tx.GetContext(ctx, &current, `SELECT id, tenant_id, title, template_id, content, created_at, updated_at FROM resumes WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return Resume{}, ErrNotFound
	}
	if err != nil {
		return Resume{}, err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO resume_versions (resume_id, tenant_id, title, template_id, content, created_at) VALUES (?, ?, ?, ?, ?, ?)`, current.ID, tenantID, current.Title, current.TemplateID, current.Content, time.Now().UTC()); err != nil {
		return Resume{}, err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE resumes SET title = ?, template_id = ?, content = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`, input.Title, input.TemplateID, input.Content, now, id, tenantID); err != nil {
		return Resume{}, err
	}
	if err := tx.Commit(); err != nil {
		return Resume{}, err
	}
	return r.Get(ctx, tenantID, id)
}

func (r *Repository) Delete(ctx context.Context, tenantID, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM resumes WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

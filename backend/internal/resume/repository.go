package resume

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

var (
	ErrNotFound       = errors.New("resume not found")
	ErrAvatarKeyInUse = errors.New("avatar storage key is already in use")
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (Resume, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return Resume{}, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `INSERT INTO resumes (tenant_id, title, template_id, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, input.TenantID, input.Title, input.TemplateID, input.Content, now, now)
	if err != nil {
		return Resume{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Resume{}, err
	}
	if key := avatarKey(input.Content); key != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO resume_avatar_keys (tenant_id, avatar_storage_key, resume_id) VALUES (?, ?, ?)`, input.TenantID, key, id); err != nil {
			return Resume{}, avatarKeyError(err)
		}
	}
	if err := tx.Commit(); err != nil {
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

// AvatarKeyResumeID returns the resume currently bound to an avatar key for a
// tenant. It deliberately exposes no cross-tenant information.
func (r *Repository) AvatarKeyResumeID(ctx context.Context, tenantID int64, key string) (int64, bool, error) {
	var resumeID int64
	err := r.db.GetContext(ctx, &resumeID, `SELECT resume_id FROM resume_avatar_keys WHERE tenant_id = ? AND avatar_storage_key = ?`, tenantID, key)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return resumeID, true, nil
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
	oldKey, newKey := avatarKey(current.Content), avatarKey(input.Content)
	if oldKey != newKey {
		if oldKey != "" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM resume_avatar_keys WHERE tenant_id = ? AND avatar_storage_key = ? AND resume_id = ?`, tenantID, oldKey, id); err != nil {
				return Resume{}, err
			}
		}
		if newKey != "" {
			if _, err := tx.ExecContext(ctx, `INSERT INTO resume_avatar_keys (tenant_id, avatar_storage_key, resume_id) VALUES (?, ?, ?)`, tenantID, newKey, id); err != nil {
				return Resume{}, avatarKeyError(err)
			}
		}
	} else if newKey != "" {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO resume_avatar_keys (tenant_id, avatar_storage_key, resume_id) VALUES (?, ?, ?)`, tenantID, newKey, id); err != nil {
			return Resume{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Resume{}, err
	}
	return r.Get(ctx, tenantID, id)
}

func avatarKey(content string) string {
	var document struct {
		Profile struct {
			AvatarStorageKey string `json:"avatarStorageKey"`
		} `json:"profile"`
	}
	_ = json.Unmarshal([]byte(content), &document)
	return document.Profile.AvatarStorageKey
}

func avatarKeyError(err error) error {
	if strings.Contains(strings.ToLower(err.Error()), "unique") {
		return ErrAvatarKeyInUse
	}
	return err
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

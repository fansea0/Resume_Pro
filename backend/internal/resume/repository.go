package resume

import (
	"context"
	"database/sql"
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
	result, err := r.db.ExecContext(ctx, `INSERT INTO resumes (title, template_id, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, input.Title, input.TemplateID, input.Content, now, now)
	if err != nil {
		return Resume{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Resume{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repository) Get(ctx context.Context, id int64) (Resume, error) {
	var item Resume
	err := r.db.GetContext(ctx, &item, `SELECT id, title, template_id, content, created_at, updated_at FROM resumes WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Resume{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) List(ctx context.Context) ([]Resume, error) {
	items := []Resume{}
	err := r.db.SelectContext(ctx, &items, `SELECT id, title, template_id, content, created_at, updated_at FROM resumes ORDER BY updated_at DESC, id DESC`)
	return items, err
}

func (r *Repository) Update(ctx context.Context, id int64, input UpdateInput) (Resume, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return Resume{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var current Resume
	err = tx.GetContext(ctx, &current, `SELECT id, title, template_id, content, created_at, updated_at FROM resumes WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Resume{}, ErrNotFound
	}
	if err != nil {
		return Resume{}, err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO resume_versions (resume_id, title, template_id, content, created_at) VALUES (?, ?, ?, ?, ?)`, current.ID, current.Title, current.TemplateID, current.Content, time.Now().UTC()); err != nil {
		return Resume{}, err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE resumes SET title = ?, template_id = ?, content = ?, updated_at = ? WHERE id = ?`, input.Title, input.TemplateID, input.Content, now, id); err != nil {
		return Resume{}, err
	}
	if err := tx.Commit(); err != nil {
		return Resume{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM resumes WHERE id = ?`, id)
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

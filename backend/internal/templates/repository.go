// Package templates owns persistence for administrator-managed template state.
package templates

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

var ErrInvalidTemplateID = errors.New("template ID is required")

type Repository struct{ db *sqlx.DB }

func NewRepository(db *sqlx.DB) *Repository { return &Repository{db: db} }

// ListDisabled returns template IDs that administrators have disabled.
func (r *Repository) ListDisabled(ctx context.Context) ([]string, error) {
	templateIDs := make([]string, 0)
	if err := r.db.SelectContext(ctx, &templateIDs, `SELECT template_id FROM disabled_templates ORDER BY template_id`); err != nil {
		return nil, err
	}
	return templateIDs, nil
}

// Disable records a disabled template ID. Repeating the same request is safe.
func (r *Repository) Disable(ctx context.Context, templateID string) error {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return ErrInvalidTemplateID
	}
	_, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO disabled_templates (template_id, disabled_at) VALUES (?, ?)`, templateID, time.Now().UTC())
	return err
}

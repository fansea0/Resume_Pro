// Package store manages database setup.
package store

import (
	"context"

	"github.com/jmoiron/sqlx"
)

const schema = `
CREATE TABLE IF NOT EXISTS resumes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  template_id TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS resume_versions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  resume_id INTEGER NOT NULL REFERENCES resumes(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  template_id TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_resume_versions_resume_id ON resume_versions(resume_id);
`

// Migrate applies the application's idempotent SQLite schema migrations.
func Migrate(ctx context.Context, db *sqlx.DB) error {
	_, err := db.ExecContext(ctx, schema)
	return err
}

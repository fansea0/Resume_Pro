// Package store manages database setup.
package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

const schema = `
CREATE TABLE IF NOT EXISTS tenants (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  display_name TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  email TEXT NOT NULL COLLATE NOCASE UNIQUE,
  password_hash TEXT NOT NULL,
  tenant_id INTEGER NOT NULL REFERENCES tenants(id),
  is_admin INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS user_identities (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  provider_subject TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  UNIQUE (provider, provider_subject)
);
CREATE TABLE IF NOT EXISTS resumes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tenant_id INTEGER NOT NULL DEFAULT 1 REFERENCES tenants(id),
  title TEXT NOT NULL,
  template_id TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS resume_versions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  resume_id INTEGER NOT NULL REFERENCES resumes(id) ON DELETE CASCADE,
  tenant_id INTEGER NOT NULL DEFAULT 1 REFERENCES tenants(id),
  title TEXT NOT NULL,
  template_id TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_resume_versions_resume_id ON resume_versions(resume_id);
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_user_identities_user_id ON user_identities(user_id);
CREATE TABLE IF NOT EXISTS resume_avatar_keys (
  tenant_id INTEGER NOT NULL REFERENCES tenants(id),
  avatar_storage_key TEXT NOT NULL,
  resume_id INTEGER NOT NULL REFERENCES resumes(id) ON DELETE CASCADE,
  PRIMARY KEY (tenant_id, avatar_storage_key),
  UNIQUE (resume_id)
);
CREATE TABLE IF NOT EXISTS disabled_templates (
  template_id TEXT PRIMARY KEY,
  disabled_at DATETIME NOT NULL
);
`

// Migrate applies the application's idempotent SQLite schema migrations.
func Migrate(ctx context.Context, db *sqlx.DB) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return err
	}

	// A stable bootstrap tenant makes this migration safe for databases created
	// before authentication and for older repository callers during rollout.
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tenants (id, display_name, created_at, updated_at) VALUES (1, 'Legacy workspace', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	if err := addColumnIfMissing(ctx, tx, "users", "is_admin", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(ctx, tx, "resumes", "tenant_id", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := addColumnIfMissing(ctx, tx, "resume_versions", "tenant_id", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE resumes SET tenant_id = 1 WHERE tenant_id IS NULL OR tenant_id = 0`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE resume_versions SET tenant_id = COALESCE((SELECT tenant_id FROM resumes WHERE resumes.id = resume_versions.resume_id), 1) WHERE tenant_id IS NULL OR tenant_id = 0`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_resumes_tenant_id ON resumes(tenant_id);
CREATE INDEX IF NOT EXISTS idx_resume_versions_tenant_id ON resume_versions(tenant_id);`); err != nil {
		return err
	}
	if err := backfillAvatarKeys(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// backfillAvatarKeys deterministically binds the lowest resume ID when legacy
// rows duplicate a key; later duplicates remain unbound and are repaired only
// after users select a different key.
func backfillAvatarKeys(ctx context.Context, tx *sqlx.Tx) error {
	rows, err := tx.QueryxContext(ctx, `SELECT id, tenant_id, content FROM resumes ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, tenantID int64
		var content string
		if err := rows.Scan(&id, &tenantID, &content); err != nil {
			return err
		}
		var doc struct {
			Profile struct {
				Key string `json:"avatarStorageKey"`
			} `json:"profile"`
		}
		if json.Unmarshal([]byte(content), &doc) != nil {
			continue
		}
		parsed, err := uuid.Parse(doc.Profile.Key)
		if err != nil || doc.Profile.Key == "" || parsed.String() != doc.Profile.Key {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO resume_avatar_keys (tenant_id, avatar_storage_key, resume_id) VALUES (?, ?, ?)`, tenantID, doc.Profile.Key, id); err != nil {
			return err
		}
	}
	return rows.Err()
}

func addColumnIfMissing(ctx context.Context, tx *sqlx.Tx, table, column, definition string) error {
	rows, err := tx.QueryxContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid          int
			name         string
			columnType   string
			notNull      int
			defaultValue any
			primaryKey   int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}

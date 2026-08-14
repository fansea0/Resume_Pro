package auth_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/store"
)

func TestRepositoryCreateUser(t *testing.T) {
	t.Parallel()

	db, err := sqlx.Open("sqlite", "file:auth-repository-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repository := auth.NewRepository(db)
	user, err := repository.CreateUser(ctx, auth.CreateUserInput{
		Email:    "  Ada.Lovelace@Example.COM ",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.Email != "ada.lovelace@example.com" {
		t.Fatalf("normalized email = %q", user.Email)
	}
	if user.TenantID == 0 {
		t.Fatal("tenant ID was not assigned")
	}
	if !auth.CheckPassword(user.PasswordHash, "correct horse battery staple") {
		t.Fatal("stored password hash did not verify")
	}
	if !strings.HasPrefix(user.PasswordHash, "$2") {
		t.Fatalf("password hash = %q, want bcrypt", user.PasswordHash)
	}
	if cost, err := bcrypt.Cost([]byte(user.PasswordHash)); err != nil || cost != 12 {
		t.Fatalf("bcrypt cost = %d, error = %v; want cost 12", cost, err)
	}

	var identity struct {
		Provider string `db:"provider"`
		Subject  string `db:"provider_subject"`
		UserID   int64  `db:"user_id"`
	}
	if err := db.GetContext(ctx, &identity, `SELECT provider, provider_subject, user_id FROM user_identities WHERE user_id = ?`, user.ID); err != nil {
		t.Fatalf("load password identity: %v", err)
	}
	if identity.Provider != "password" || identity.Subject != user.Email || identity.UserID != user.ID {
		t.Fatalf("unexpected identity: %#v", identity)
	}

	if _, err := repository.CreateUser(ctx, auth.CreateUserInput{Email: "ADA.LOVELACE@example.com", Password: "another secure password"}); err != auth.ErrEmailAlreadyRegistered {
		t.Fatalf("duplicate normalized email error = %v, want %v", err, auth.ErrEmailAlreadyRegistered)
	}

	var tenantID int64
	if err := db.GetContext(ctx, &tenantID, `SELECT tenant_id FROM resumes LIMIT 1`); err == nil {
		t.Fatalf("unexpected resume row with tenant ID %d", tenantID)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO resumes (title, template_id, content, created_at, updated_at) VALUES ('legacy-compatible', 'minimal', '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert resume: %v", err)
	}
	if err := db.GetContext(ctx, &tenantID, `SELECT tenant_id FROM resumes WHERE title = 'legacy-compatible'`); err != nil {
		t.Fatalf("read resume tenant ID: %v", err)
	}
	if tenantID == 0 {
		t.Fatal("resume did not receive a tenant ID")
	}
}

func TestRepositoryNewUsersAreNotAdministrators(t *testing.T) {
	t.Parallel()

	db, err := sqlx.Open("sqlite", "file:auth-new-admin-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	user, err := auth.NewRepository(db).CreateUser(ctx, auth.CreateUserInput{
		Email: "member@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.IsAdmin {
		t.Fatal("new user unexpectedly has administrator role")
	}
}

func TestRepositorySetAdministrators(t *testing.T) {
	t.Parallel()

	db, err := sqlx.Open("sqlite", "file:auth-set-admin-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repository := auth.NewRepository(db)
	admin, err := repository.CreateUser(ctx, auth.CreateUserInput{Email: "admin@example.com", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatalf("create administrator candidate: %v", err)
	}
	member, err := repository.CreateUser(ctx, auth.CreateUserInput{Email: "member@example.com", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatalf("create member: %v", err)
	}

	if err := repository.SetAdministrators(ctx, []string{"  ADMIN@EXAMPLE.COM  "}); err != nil {
		t.Fatalf("set administrators: %v", err)
	}
	admin, err = repository.GetUserByID(ctx, admin.ID)
	if err != nil {
		t.Fatalf("reload administrator: %v", err)
	}
	member, err = repository.GetUserByID(ctx, member.ID)
	if err != nil {
		t.Fatalf("reload member: %v", err)
	}
	if !admin.IsAdmin {
		t.Fatal("configured administrator was not promoted")
	}
	if member.IsAdmin {
		t.Fatal("unconfigured user was promoted")
	}

	if err := repository.SetAdministrators(ctx, nil); err != nil {
		t.Fatalf("clear administrators: %v", err)
	}
	admin, err = repository.GetUserByID(ctx, admin.ID)
	if err != nil {
		t.Fatalf("reload cleared administrator: %v", err)
	}
	if admin.IsAdmin {
		t.Fatal("empty administrator configuration left a user promoted")
	}
}

func TestMigrationBackfillsLegacyResumeTenants(t *testing.T) {
	t.Parallel()

	db, err := sqlx.Open("sqlite", "file:auth-legacy-migration-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	legacySchema := `
CREATE TABLE resumes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  template_id TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE TABLE resume_versions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  resume_id INTEGER NOT NULL REFERENCES resumes(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  template_id TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at DATETIME NOT NULL
);`
	if _, err := db.ExecContext(ctx, legacySchema); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO resumes (title, template_id, content, created_at, updated_at) VALUES ('old', 'minimal', '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert legacy resume: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO resume_versions (resume_id, title, template_id, content, created_at) VALUES (1, 'old', 'minimal', '{}', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert legacy version: %v", err)
	}

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate legacy database: %v", err)
	}
	for _, table := range []string{"resumes", "resume_versions"} {
		var tenantID int64
		if err := db.GetContext(ctx, &tenantID, `SELECT tenant_id FROM `+table+` WHERE id = 1`); err != nil {
			t.Fatalf("read %s tenant ID: %v", table, err)
		}
		if tenantID != 1 {
			t.Fatalf("%s tenant ID = %d, want bootstrap tenant 1", table, tenantID)
		}
	}
}

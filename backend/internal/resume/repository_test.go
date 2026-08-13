package resume_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/store"
)

func TestRepositoryCreateUpdate(t *testing.T) {
	t.Parallel()

	db, err := sqlx.Open("sqlite", "file:resume-repository-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repository := resume.NewRepository(db)
	tenantID := createTenant(t, db, "Create/update tenant")
	created, err := repository.Create(context.Background(), resume.CreateInput{
		TenantID:   tenantID,
		Title:      "Backend Engineer",
		TemplateID: "minimal",
		Content:    jsonContent("Ada Lovelace"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	fetched, err := repository.Get(context.Background(), tenantID, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if fetched.ID != created.ID || fetched.Title != "Backend Engineer" || fetched.TemplateID != "minimal" || fetched.Content != jsonContent("Ada Lovelace") {
		t.Fatalf("unexpected fetched resume: %#v", fetched)
	}

	updated, err := repository.Update(context.Background(), tenantID, created.ID, resume.UpdateInput{
		Title:      "Senior Backend Engineer",
		TemplateID: "business",
		Content:    jsonContent("Grace Hopper"),
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != "Senior Backend Engineer" || updated.TemplateID != "business" || updated.Content != jsonContent("Grace Hopper") {
		t.Fatalf("unexpected updated resume: %#v", updated)
	}

	var versions int
	if err := db.Get(&versions, "SELECT COUNT(*) FROM resume_versions WHERE resume_id = ?", created.ID); err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if versions != 1 {
		t.Fatalf("version count = %d, want 1", versions)
	}
	var versionTenantID int64
	if err := db.Get(&versionTenantID, "SELECT tenant_id FROM resume_versions WHERE resume_id = ?", created.ID); err != nil {
		t.Fatalf("get version tenant: %v", err)
	}
	if versionTenantID != tenantID {
		t.Fatalf("version tenant ID = %d, want %d", versionTenantID, tenantID)
	}
}

func TestRepositoryTenantIsolation(t *testing.T) {
	t.Parallel()

	db, err := sqlx.Open("sqlite", "file:resume-tenant-isolation-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repository := resume.NewRepository(db)
	tenantA := createTenant(t, db, "Tenant A")
	tenantB := createTenant(t, db, "Tenant B")
	owned, err := repository.Create(ctx, resume.CreateInput{TenantID: tenantA, Title: "Private", TemplateID: "minimal", Content: jsonContent("A")})
	if err != nil {
		t.Fatalf("create tenant A resume: %v", err)
	}
	if _, err := repository.Create(ctx, resume.CreateInput{TenantID: tenantB, Title: "Other", TemplateID: "minimal", Content: jsonContent("B")}); err != nil {
		t.Fatalf("create tenant B resume: %v", err)
	}

	items, err := repository.List(ctx, tenantA)
	if err != nil {
		t.Fatalf("list tenant A: %v", err)
	}
	if len(items) != 1 || items[0].ID != owned.ID {
		t.Fatalf("tenant A list = %#v, want only resume %d", items, owned.ID)
	}

	if _, err := repository.Get(ctx, tenantB, owned.ID); err != resume.ErrNotFound {
		t.Fatalf("cross-tenant get error = %v, want %v", err, resume.ErrNotFound)
	}
	if _, err := repository.Update(ctx, tenantB, owned.ID, resume.UpdateInput{Title: "Stolen", TemplateID: "business", Content: jsonContent("B")}); err != resume.ErrNotFound {
		t.Fatalf("cross-tenant update error = %v, want %v", err, resume.ErrNotFound)
	}
	if err := repository.Delete(ctx, tenantB, owned.ID); err != resume.ErrNotFound {
		t.Fatalf("cross-tenant delete error = %v, want %v", err, resume.ErrNotFound)
	}

	stillOwned, err := repository.Get(ctx, tenantA, owned.ID)
	if err != nil {
		t.Fatalf("get own resume after cross-tenant mutations: %v", err)
	}
	if stillOwned.Title != "Private" {
		t.Fatalf("resume title = %q, want unchanged", stillOwned.Title)
	}
	if err := repository.Delete(ctx, tenantA, owned.ID); err != nil {
		t.Fatalf("delete own resume: %v", err)
	}
	if _, err := repository.Get(ctx, tenantA, owned.ID); err != resume.ErrNotFound {
		t.Fatalf("get deleted resume error = %v, want %v", err, resume.ErrNotFound)
	}
}

func createTenant(t *testing.T, db *sqlx.DB, displayName string) int64 {
	t.Helper()
	result, err := db.Exec(`INSERT INTO tenants (display_name, created_at, updated_at) VALUES (?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, displayName)
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("tenant ID: %v", err)
	}
	return id
}

func jsonContent(name string) string {
	return `{"profile":{"name":"` + name + `"}}`
}

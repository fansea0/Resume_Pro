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
	created, err := repository.Create(context.Background(), resume.CreateInput{
		Title:      "Backend Engineer",
		TemplateID: "minimal",
		Content:    jsonContent("Ada Lovelace"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	fetched, err := repository.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if fetched.ID != created.ID || fetched.Title != "Backend Engineer" || fetched.TemplateID != "minimal" || fetched.Content != jsonContent("Ada Lovelace") {
		t.Fatalf("unexpected fetched resume: %#v", fetched)
	}

	updated, err := repository.Update(context.Background(), created.ID, resume.UpdateInput{
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
}

func jsonContent(name string) string {
	return `{"profile":{"name":"` + name + `"}}`
}

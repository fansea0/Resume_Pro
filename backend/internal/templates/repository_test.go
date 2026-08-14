package templates

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"resumepro/backend/internal/store"
)

func TestRepositoryDisablesTemplateIdempotently(t *testing.T) {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}

	repository := NewRepository(db)
	if err := repository.Disable(ctx, "minimal"); err != nil {
		t.Fatalf("disable template: %v", err)
	}
	if err := repository.Disable(ctx, "minimal"); err != nil {
		t.Fatalf("disable template a second time: %v", err)
	}
	disabled, err := repository.ListDisabled(ctx)
	if err != nil {
		t.Fatalf("list disabled templates: %v", err)
	}
	if len(disabled) != 1 || disabled[0] != "minimal" {
		t.Fatalf("disabled templates = %#v, want [minimal]", disabled)
	}
}

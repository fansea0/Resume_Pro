package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"resumepro/backend/internal/store"
)

func TestOpenSQLiteConfiguresEveryPooledConnectionAndKeepsMigrationExplicit(t *testing.T) {
	db, err := store.OpenSQLite(filepath.Join(t.TempDir(), "policy.db"))
	require.NoError(t, err)
	db.SetMaxOpenConns(2)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	ctx := context.Background()
	first, err := db.Connx(ctx)
	require.NoError(t, err)
	defer first.Close()
	second, err := db.Connx(ctx)
	require.NoError(t, err)
	defer second.Close()

	for _, connection := range []*sqlx.Conn{first, second} {
		var timeout int
		require.NoError(t, connection.GetContext(ctx, &timeout, `PRAGMA busy_timeout`))
		assert.Equal(t, 5000, timeout)
		var tableCount int
		require.NoError(t, connection.GetContext(ctx, &tableCount, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'resumes'`))
		assert.Zero(t, tableCount, "opening the database must not implicitly run migrations")
	}

	require.NoError(t, first.Close())
	require.NoError(t, second.Close())
	require.NoError(t, store.Migrate(ctx, db))
	var tenantCount int
	require.NoError(t, db.GetContext(ctx, &tenantCount, `SELECT COUNT(*) FROM tenants WHERE id = 1`))
	assert.Equal(t, 1, tenantCount)
}

package targetjob_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"resumepro/backend/internal/store"
	"resumepro/backend/internal/targetjob"
)

func newTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, store.Migrate(context.Background(), db))
	return db
}

func TestRepository_CreateAndGet(t *testing.T) {
	repo := targetjob.NewRepository(newTestDB(t))
	ctx := context.Background()

	tj, err := repo.Create(ctx, 1, targetjob.CreateInput{
		Company:      "字节",
		Position:     "后端",
		Requirements: "负责 ToB 营销中台后端架构",
	})
	require.NoError(t, err)
	assert.NotZero(t, tj.ID)
	assert.Equal(t, "字节", tj.Company)

	got, err := repo.Get(ctx, 1, tj.ID)
	require.NoError(t, err)
	assert.Equal(t, tj.ID, got.ID)
	assert.Equal(t, "负责 ToB 营销中台后端架构", got.Requirements)
}

func TestRepository_Get_TenantIsolation(t *testing.T) {
	repo := targetjob.NewRepository(newTestDB(t))
	ctx := context.Background()

	tj, err := repo.Create(ctx, 1, targetjob.CreateInput{Requirements: "JD only"})
	require.NoError(t, err)

	_, err = repo.Get(ctx, 999, tj.ID)
	assert.ErrorIs(t, err, targetjob.ErrNotFound)
}

func TestRepository_List_OrderedByUpdatedAtDesc(t *testing.T) {
	repo := targetjob.NewRepository(newTestDB(t))
	ctx := context.Background()

	a, err := repo.Create(ctx, 1, targetjob.CreateInput{Requirements: "first"})
	require.NoError(t, err)
	b, err := repo.Create(ctx, 1, targetjob.CreateInput{Requirements: "second"})
	require.NoError(t, err)
	_, err = repo.Update(ctx, 1, a.ID, targetjob.UpdateInput{Requirements: "first updated"})
	require.NoError(t, err)

	list, err := repo.List(ctx, 1)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, a.ID, list[0].ID)
	assert.Equal(t, b.ID, list[1].ID)
}

func TestRepository_Update(t *testing.T) {
	repo := targetjob.NewRepository(newTestDB(t))
	ctx := context.Background()

	tj, err := repo.Create(ctx, 1, targetjob.CreateInput{Requirements: "old"})
	require.NoError(t, err)
	updated, err := repo.Update(ctx, 1, tj.ID, targetjob.UpdateInput{
		Company:      "新公司",
		Position:     "新职位",
		Requirements: "新 JD",
	})
	require.NoError(t, err)
	assert.Equal(t, "新公司", updated.Company)
	assert.Equal(t, "新职位", updated.Position)
	assert.Equal(t, "新 JD", updated.Requirements)
}

func TestRepository_Delete(t *testing.T) {
	repo := targetjob.NewRepository(newTestDB(t))
	ctx := context.Background()

	tj, err := repo.Create(ctx, 1, targetjob.CreateInput{Requirements: "to delete"})
	require.NoError(t, err)
	require.NoError(t, repo.Delete(ctx, 1, tj.ID))

	_, err = repo.Get(ctx, 1, tj.ID)
	assert.ErrorIs(t, err, targetjob.ErrNotFound)
}

func TestRepository_CountByTargetJob(t *testing.T) {
	repo := targetjob.NewRepository(newTestDB(t))
	ctx := context.Background()

	tj, err := repo.Create(ctx, 1, targetjob.CreateInput{Requirements: "test"})
	require.NoError(t, err)
	n, err := repo.CountByTargetJob(ctx, 1, tj.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

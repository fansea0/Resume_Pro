package targetjob_test

import (
	"context"
	"strconv"
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
	db, err := sqlx.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, store.Migrate(context.Background(), db))
	return db
}

func createTenant(t *testing.T, db *sqlx.DB, name string) int64 {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO tenants (display_name, created_at, updated_at)
		VALUES (?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, name)
	require.NoError(t, err)
	id, err := result.LastInsertId()
	require.NoError(t, err)
	return id
}

func createResumeReference(t *testing.T, db *sqlx.DB, tenantID, targetJobID int64) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO resumes (tenant_id, title, template_id, content, created_at, updated_at)
		VALUES (?, 'resume', 'classic', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		tenantID, `{"targetJobId":`+strconv.FormatInt(targetJobID, 10)+`}`)
	require.NoError(t, err)
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

func TestRepository_List_TenantIsolation(t *testing.T) {
	db := newTestDB(t)
	repo := targetjob.NewRepository(db)
	ctx := context.Background()
	tenant2 := createTenant(t, db, "Tenant two")

	first, err := repo.Create(ctx, 1, targetjob.CreateInput{Requirements: "tenant one"})
	require.NoError(t, err)
	_, err = repo.Create(ctx, tenant2, targetjob.CreateInput{Requirements: "tenant two"})
	require.NoError(t, err)

	list, err := repo.List(ctx, 1)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, first.ID, list[0].ID)
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

func TestRepository_Update_TenantIsolation(t *testing.T) {
	db := newTestDB(t)
	repo := targetjob.NewRepository(db)
	ctx := context.Background()
	tenant2 := createTenant(t, db, "Tenant two")

	tj, err := repo.Create(ctx, 1, targetjob.CreateInput{Requirements: "unchanged"})
	require.NoError(t, err)
	_, err = repo.Update(ctx, tenant2, tj.ID, targetjob.UpdateInput{Requirements: "cross-tenant update"})
	assert.ErrorIs(t, err, targetjob.ErrNotFound)

	got, err := repo.Get(ctx, 1, tj.ID)
	require.NoError(t, err)
	assert.Equal(t, "unchanged", got.Requirements)
}

func TestRepository_Delete(t *testing.T) {
	repo := targetjob.NewRepository(newTestDB(t))
	ctx := context.Background()

	tj, err := repo.Create(ctx, 1, targetjob.CreateInput{Requirements: "to delete"})
	require.NoError(t, err)
	cleared, err := repo.Delete(ctx, 1, tj.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), cleared)

	_, err = repo.Get(ctx, 1, tj.ID)
	assert.ErrorIs(t, err, targetjob.ErrNotFound)
}

func TestRepository_Delete_ClearsOnlySameTenantResumeReferences(t *testing.T) {
	db := newTestDB(t)
	repo := targetjob.NewRepository(db)
	ctx := context.Background()
	tenant2 := createTenant(t, db, "Tenant two")

	tj, err := repo.Create(ctx, 1, targetjob.CreateInput{Requirements: "target requirement"})
	require.NoError(t, err)
	ownedContent := `{"version":2,"targetJobId":` + strconv.FormatInt(tj.ID, 10) + `,"profile":{"name":"Ada"},"sections":[{"id":"summary"}]}`
	otherTenantContent := `{"version":2,"targetJobId":` + strconv.FormatInt(tj.ID, 10) + `,"profile":{"name":"Grace"}}`
	ownedResult, err := db.Exec(`
		INSERT INTO resumes (tenant_id, title, template_id, content, created_at, updated_at)
		VALUES (?, 'owned', 'classic', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, 1, ownedContent)
	require.NoError(t, err)
	ownedID, err := ownedResult.LastInsertId()
	require.NoError(t, err)
	otherResult, err := db.Exec(`
		INSERT INTO resumes (tenant_id, title, template_id, content, created_at, updated_at)
		VALUES (?, 'other', 'classic', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, tenant2, otherTenantContent)
	require.NoError(t, err)
	otherID, err := otherResult.LastInsertId()
	require.NoError(t, err)
	malformedResult, err := db.Exec(`
		INSERT INTO resumes (tenant_id, title, template_id, content, created_at, updated_at)
		VALUES (?, 'malformed', 'classic', '{bad json', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, 1)
	require.NoError(t, err)
	malformedID, err := malformedResult.LastInsertId()
	require.NoError(t, err)

	cleared, err := repo.Delete(ctx, 1, tj.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), cleared)

	var gotOwned, gotOther, gotMalformed string
	require.NoError(t, db.Get(&gotOwned, `SELECT content FROM resumes WHERE id = ?`, ownedID))
	require.NoError(t, db.Get(&gotOther, `SELECT content FROM resumes WHERE id = ?`, otherID))
	require.NoError(t, db.Get(&gotMalformed, `SELECT content FROM resumes WHERE id = ?`, malformedID))
	assert.JSONEq(t, `{"version":2,"profile":{"name":"Ada"},"sections":[{"id":"summary"}]}`, gotOwned)
	assert.JSONEq(t, otherTenantContent, gotOther)
	assert.Equal(t, `{bad json`, gotMalformed)
}

func TestRepository_Delete_TenantIsolation(t *testing.T) {
	db := newTestDB(t)
	repo := targetjob.NewRepository(db)
	ctx := context.Background()
	tenant2 := createTenant(t, db, "Tenant two")

	tj, err := repo.Create(ctx, 1, targetjob.CreateInput{Requirements: "must remain"})
	require.NoError(t, err)
	createResumeReference(t, db, tenant2, tj.ID)
	_, err = repo.Delete(ctx, tenant2, tj.ID)
	assert.ErrorIs(t, err, targetjob.ErrNotFound)

	got, err := repo.Get(ctx, 1, tj.ID)
	require.NoError(t, err)
	assert.Equal(t, tj.ID, got.ID)
	var content string
	require.NoError(t, db.Get(&content, `SELECT content FROM resumes WHERE tenant_id = ?`, tenant2))
	assert.JSONEq(t, `{"targetJobId":`+strconv.FormatInt(tj.ID, 10)+`}`, content)
}

func TestRepository_CountByTargetJob(t *testing.T) {
	db := newTestDB(t)
	repo := targetjob.NewRepository(db)
	ctx := context.Background()
	tenant2 := createTenant(t, db, "Tenant two")

	tj, err := repo.Create(ctx, 1, targetjob.CreateInput{Requirements: "test"})
	require.NoError(t, err)
	createResumeReference(t, db, 1, tj.ID)
	createResumeReference(t, db, tenant2, tj.ID)

	n, err := repo.CountByTargetJob(ctx, 1, tj.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	n, err = repo.CountByTargetJob(ctx, tenant2, tj.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

package targetjob_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/store"
	"resumepro/backend/internal/targetjob"
)

func TestResumeWriteAndTargetDeleteWaitForTheOtherSQLiteWriter(t *testing.T) {
	t.Run("resume write commits first and delete clears its reference", func(t *testing.T) {
		db1, db2 := openContendedDatabase(t)
		ctx := context.Background()
		target, err := targetjob.NewRepository(db1).Create(ctx, 1, targetjob.CreateInput{Requirements: "Build reliable backend services"})
		require.NoError(t, err)
		tx, err := db1.BeginTxx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback() })
		_, err = tx.ExecContext(ctx, `UPDATE target_jobs SET updated_at = updated_at WHERE id = ? AND tenant_id = 1`, target.ID)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `INSERT INTO resumes (tenant_id, title, template_id, content, created_at, updated_at) VALUES (1, 'Concurrent', 'minimal', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, targetContent(target.ID))
		require.NoError(t, err)

		result := make(chan deleteResult, 1)
		started := make(chan struct{})
		go func() {
			close(started)
			cleared, deleteErr := targetjob.NewRepository(db2).Delete(ctx, 1, target.ID)
			result <- deleteResult{cleared: cleared, err: deleteErr}
		}()
		<-started
		assertStillWaiting(t, result)
		require.NoError(t, tx.Commit())

		deleted := receiveDeleteResult(t, result)
		require.NoError(t, deleted.err)
		assert.Equal(t, int64(1), deleted.cleared)
		var content string
		require.NoError(t, db1.GetContext(ctx, &content, `SELECT content FROM resumes WHERE tenant_id = 1`))
		assert.JSONEq(t, `{"schemaVersion":2}`, content)
	})

	t.Run("target delete commits first and resume write rejects the stale reference", func(t *testing.T) {
		db1, db2 := openContendedDatabase(t)
		ctx := context.Background()
		target, err := targetjob.NewRepository(db1).Create(ctx, 1, targetjob.CreateInput{Requirements: "Build reliable backend services"})
		require.NoError(t, err)
		tx, err := db1.BeginTxx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback() })
		_, err = tx.ExecContext(ctx, `DELETE FROM target_jobs WHERE id = ? AND tenant_id = 1`, target.ID)
		require.NoError(t, err)

		result := make(chan error, 1)
		started := make(chan struct{})
		go func() {
			close(started)
			_, createErr := resume.NewRepository(db2).Create(ctx, resume.CreateInput{
				TenantID: 1, TargetJobID: &target.ID, Title: "Concurrent", TemplateID: "minimal", Content: targetContent(target.ID),
			})
			result <- createErr
		}()
		<-started
		assertStillWaiting(t, result)
		require.NoError(t, tx.Commit())

		select {
		case err := <-result:
			require.ErrorIs(t, err, resume.ErrTargetJobNotFound)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for resume create")
		}
		var count int
		require.NoError(t, db1.GetContext(ctx, &count, `SELECT COUNT(*) FROM resumes`))
		assert.Zero(t, count)
	})
}

type deleteResult struct {
	cleared int64
	err     error
}

func openContendedDatabase(t *testing.T) (*sqlx.DB, *sqlx.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "contention.db")
	db1, err := sqlx.Open("sqlite", path)
	require.NoError(t, err)
	db2, err := sqlx.Open("sqlite", path)
	require.NoError(t, err)
	db1.SetMaxOpenConns(1)
	db2.SetMaxOpenConns(1)
	t.Cleanup(func() {
		require.NoError(t, errors.Join(db1.Close(), db2.Close()))
	})
	require.NoError(t, store.Migrate(context.Background(), db1))
	return db1, db2
}

func targetContent(targetJobID int64) string {
	return `{"schemaVersion":2,"targetJobId":` + fmt.Sprint(targetJobID) + `}`
}

func assertStillWaiting[T any](t *testing.T, result <-chan T) {
	t.Helper()
	select {
	case <-result:
		t.Fatal("contending writer returned before the first transaction committed")
	case <-time.After(100 * time.Millisecond):
	}
}

func receiveDeleteResult(t *testing.T, result <-chan deleteResult) deleteResult {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for target-job delete")
		return deleteResult{}
	}
}

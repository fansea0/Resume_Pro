package store

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func newTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := OpenSQLite(":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, Migrate(context.Background(), db))
	return db
}

func TestMigrateCreatesInterviewTables(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var n int
	if err := db.GetContext(ctx, &n, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='interviews'`); err != nil || n != 1 {
		t.Fatalf("interviews table missing: n=%d err=%v", n, err)
	}
	if err := db.GetContext(ctx, &n, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='interview_questions'`); err != nil || n != 1 {
		t.Fatalf("interview_questions table missing: n=%d err=%v", n, err)
	}
}

// TestMigrateAddsAudioStateColumnToInterviewQuestions 验证题目语音状态列存在且迁移幂等。
func TestMigrateAddsAudioStateColumnToInterviewQuestions(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var columnType string
	if err := db.GetContext(ctx, &columnType, `SELECT type FROM pragma_table_info('interview_questions') WHERE name = 'audio_state'`); err != nil || columnType == "" {
		t.Fatalf("audio_state column missing: type=%q err=%v", columnType, err)
	}
	// 迁移幂等：重复执行不报错。
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

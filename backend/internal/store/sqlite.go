package store

import (
	"context"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

const sqliteBusyTimeoutMilliseconds = 5000

// OpenSQLite applies the writer wait policy to every pooled connection. The
// migration remains explicit so callers keep control of when schema work runs.
func OpenSQLite(path string) (*sqlx.DB, error) {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	timeout := strconv.Itoa(sqliteBusyTimeoutMilliseconds)
	return sqlx.Open("sqlite", path+separator+"_pragma=busy_timeout("+timeout+")")
}

// BeginSQLiteWriteTx is a transaction-level fallback for repositories also
// used with externally constructed database pools.
func BeginSQLiteWriteTx(ctx context.Context, db *sqlx.DB) (*sqlx.Tx, error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	timeout := strconv.Itoa(sqliteBusyTimeoutMilliseconds)
	if _, err := tx.ExecContext(ctx, "PRAGMA busy_timeout = "+timeout); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

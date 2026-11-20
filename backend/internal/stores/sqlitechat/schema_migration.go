package sqlitechat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

type SchemaUpgradeResult struct {
	FromVersion int    `json:"fromVersion"`
	ToVersion   int    `json:"toVersion"`
	BackupPath  string `json:"backupPath,omitempty"`
}

// UpgradeSchema runs only in the offline migration command. The exclusive
// store lease keeps the server stopped for the backup and conversion. DDL,
// backfill, validation, and the version change share one SQLite transaction:
// interruption leaves v1 intact and the next invocation safely retries.
func UpgradeSchema(ctx context.Context, root string) (result SchemaUpgradeResult, err error) {
	path := filepath.Join(root, Filename)
	if _, err = os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return result, nil
	} else if err != nil {
		return result, err
	}
	lock, err := acquireStoreLock(root)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, lock.Close()) }()
	db, err := openSchemaDatabase(path, false)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&result.FromVersion); err != nil {
		return result, err
	}
	result.ToVersion = result.FromVersion
	if result.FromVersion == 0 || result.FromVersion == schemaVersion {
		return result, nil
	}
	if result.FromVersion != 1 {
		return result, fmt.Errorf("unsupported conversation schema %d", result.FromVersion)
	}
	result.BackupPath, err = backupSchemaDatabase(ctx, root, db)
	if err != nil {
		return result, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err = upgradeSchemaV1(ctx, tx); err != nil {
		return result, fmt.Errorf("upgrade conversation schema (v1 retained; backup %s): %w", result.BackupPath, err)
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	result.ToVersion = schemaVersion
	return result, nil
}

// SchemaVersion is read-only and does not take the exclusive runtime lease.
// Deployment uses it to check rollback compatibility against the actual DB.
func SchemaVersion(ctx context.Context, root string) (int, error) {
	path := filepath.Join(root, Filename)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	db, err := openSchemaDatabase(path, true)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var version int
	err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	return version, err
}

func openSchemaDatabase(path string, readOnly bool) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "rw")
	if readOnly {
		q.Set("mode", "ro")
		q.Add("_pragma", "query_only(ON)")
	}
	for _, pragma := range []string{"busy_timeout(5000)", "foreign_keys(ON)", "synchronous(FULL)", "trusted_schema(OFF)"} {
		q.Add("_pragma", pragma)
	}
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err == nil {
		db.SetMaxOpenConns(1)
	}
	return db, err
}

package sqlitechat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"
)

// backupSchemaDatabase retains the path after creation, even when a later
// backup or durability check fails, so the upgrade result names the artifact.
func backupSchemaDatabase(ctx context.Context, root string, db *sql.DB) (string, error) {
	// SQLite performs the backup, including committed WAL contents. Never copy
	// just the main file of an open WAL database.
	backup, err := os.CreateTemp(root, "conversations.before-v2-"+time.Now().UTC().Format("20060102T150405")+"-*.sqlite")
	if err != nil {
		return "", err
	}
	path := backup.Name()
	if err = backup.Close(); err != nil {
		return path, err
	}
	if _, err = db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return path, fmt.Errorf("back up conversations: %w", err)
	}
	backupDB, err := openSchemaDatabase(path, true)
	if err != nil {
		return path, err
	}
	err = verifySchemaDatabase(ctx, backupDB)
	err = errors.Join(err, backupDB.Close())
	if err != nil {
		return path, fmt.Errorf("verify conversation backup: %w", err)
	}
	// VACUUM INTO does not sync the destination on every SQLite version. Sync
	// the verified backup and its directory before changing the source database.
	backup, err = os.Open(path)
	if err != nil {
		return path, err
	}
	err = errors.Join(backup.Sync(), backup.Close())
	if err != nil {
		return path, err
	}
	dir, err := os.Open(root)
	if err != nil {
		return path, err
	}
	err = errors.Join(dir.Sync(), dir.Close())
	if err != nil {
		return path, err
	}
	return path, nil
}

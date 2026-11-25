package chatmigration

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"

	"github.com/futrx-com/remote.futrx.com/internal/stores/sqlitechat"
)

// Status is read-only and may be called while the backend owns the store.
// Errors are deliberately not interpreted as permission to restore JSONL.
func Status(ctx context.Context, root string) (string, error) {
	path := filepath.Join(root, sqlitechat.Filename)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return "legacy", nil
	} else if err != nil {
		return "", err
	}
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(ON)")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return "", err
	}
	defer db.Close()
	var status string
	err = db.QueryRowContext(ctx, `SELECT status FROM conversation_migration WHERE id=1`).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "importing", nil
	}
	return status, err
}

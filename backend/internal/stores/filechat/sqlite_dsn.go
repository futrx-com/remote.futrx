package filechat

import (
	"net/url"
)

// sqliteDSN builds the hardened connection string shared by every SQLite
// database this package owns: WAL for concurrent readers, immediate write
// transactions, a busy timeout, and no trusted schema.
func sqliteDSN(path string) string {
	databaseURL := &url.URL{Scheme: "file", Path: path}
	query := databaseURL.Query()
	query.Set("_busy_timeout", "5000")
	query.Set("_defensive", "1")
	query.Set("_dqs", "0")
	query.Set("_foreign_keys", "on")
	query.Set("_journal_mode", "WAL")
	query.Set("_synchronous", "NORMAL")
	query.Set("_txlock", "immediate")
	query.Add("_pragma", "trusted_schema(OFF)")
	databaseURL.RawQuery = query.Encode()
	return databaseURL.String()
}

// Package filesessions is file-backed storage for per-account session
// registry state: SecurityPreferences (single-active-session, history,
// recovery-code-alert toggles), the active session id, bounded sign-in
// history, and any pending security alert. One JSON file per account at
// <dataDir>/sessions/<sha256-hex(email)>.json, mode 0600. An account that
// has never turned on any of the three preference flags simply has no file
// - Get returns (nil, nil), which is the correct "nothing enabled" state,
// not an error.
package filesessions

import (
	"path/filepath"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/accountrecords"
)

var _ serviceauth.SessionRegistryStore = (*Store)(nil)

const directory = "sessions"

type Store struct {
	*accountrecords.Store[serviceauth.SessionRegistryRecord]
	root string
}

func New(dataDir string) (*Store, error) {
	records, err := accountrecords.New[serviceauth.SessionRegistryRecord](
		dataDir,
		directory,
		"session registry record",
	)
	if err != nil {
		return nil, err
	}
	return &Store{
		Store: records,
		root:  filepath.Join(dataDir, directory),
	}, nil
}

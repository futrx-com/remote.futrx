// Package filetwofactor is file-backed storage for per-account TOTP
// enrollment. One JSON file per enrolled account at
// <dataDir>/twofactor/<sha256-hex(email)>.json, mode 0600. An account that
// has never enrolled simply has no file - Get returns (nil, nil), which is
// the correct "2FA not enabled" state, not an error.
package filetwofactor

import (
	"path/filepath"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/accountrecords"
)

var _ serviceauth.TwoFactorStore = (*Store)(nil)

const directory = "twofactor"

type Store struct {
	*accountrecords.Store[serviceauth.TwoFactorRecord]
	root string
}

func New(dataDir string) (*Store, error) {
	records, err := accountrecords.New[serviceauth.TwoFactorRecord](
		dataDir,
		directory,
		"two-factor record",
	)
	if err != nil {
		return nil, err
	}
	return &Store{
		Store: records,
		root:  filepath.Join(dataDir, directory),
	}, nil
}

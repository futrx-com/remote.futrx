// Package chatmigration converts the frozen legacy chat files into Remote's
// authoritative SQLite store. Native harness state is deliberately untouched.
package chatmigration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/futrx-com/remote.futrx.com/internal/stores/sqlitechat"
)

type Options struct {
	BatchEvents int
	BatchBytes  int
	// Progress runs after a committed checkpoint. Cancellation is safe here.
	Progress func(sqlitechat.ImportProgress) error
}

type Result struct {
	Chats           int                             `json:"chats"`
	Events          int64                           `json:"events"`
	AlreadyComplete bool                            `json:"alreadyComplete"`
	SchemaUpgrade   *sqlitechat.SchemaUpgradeResult `json:"schemaUpgrade,omitempty"`
}

// Run must execute while the backend is stopped. The store's exclusive lease
// also rejects concurrent new-version servers and importers.
func Run(ctx context.Context, root string, options Options) (result Result, err error) {
	upgrade, err := sqlitechat.UpgradeSchema(ctx, root)
	if err != nil {
		return result, err
	}
	if upgrade.FromVersion != upgrade.ToVersion {
		result.SchemaUpgrade = &upgrade
	}
	if options.BatchEvents <= 0 {
		options.BatchEvents = 128
	}
	if options.BatchBytes <= 0 {
		options.BatchBytes = 4 * 1024 * 1024
	}
	store, err := sqlitechat.New(root)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	complete, err := store.MigrationComplete(ctx)
	if err != nil {
		return result, err
	}
	if complete {
		result.AlreadyComplete = true
		return result, removeLegacyProjection(root)
	}
	sources, manifest, err := discover(root)
	if err != nil {
		return result, err
	}
	if err := store.BeginImport(ctx, manifest); err != nil {
		return result, err
	}
	for _, src := range sources {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		p, err := importChat(ctx, store, src, options)
		if err != nil {
			return result, fmt.Errorf("migrate chat %s: %w", src.id, err)
		}
		result.Chats++
		result.Events += p.EventCount
	}
	// Recheck the complete source set, including files already imported by a
	// previous attempt. Stat includes ctime/inode so same-size replacements and
	// edits that restore mtime still invalidate a checkpoint.
	_, currentManifest, err := discover(root)
	if err != nil {
		return result, err
	}
	if currentManifest != manifest {
		return result, errors.New("legacy source changed during migration; backend must remain stopped")
	}
	if err := store.CompleteImport(ctx); err != nil {
		return result, err
	}
	return result, removeLegacyProjection(root)
}

func removeLegacyProjection(root string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(filepath.Join(root, "transcript-index.sqlite") + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

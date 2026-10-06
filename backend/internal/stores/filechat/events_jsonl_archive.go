package filechat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// jsonlArchive owns the events.jsonl file format and its atomic replacement.
// It is authoritative for jsonlLog and a best-effort rollback copy for
// sqliteLog; neither role needs the other's indexing behavior.
type jsonlArchive struct {
	store *Store
}

func newJSONLArchive(store *Store) *jsonlArchive {
	return &jsonlArchive{store: store}
}

func (a *jsonlArchive) create(id servicechat.ID) error {
	f, err := os.OpenFile(a.store.eventsPath(id), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err == nil {
		err = f.Close()
	}
	return err
}

// writeRecord appends an already-sequenced event to the archive and returns
// the exact bytes written. The sequence number comes from the caller, so a
// mirrored file always agrees with its authoritative database row.
func (a *jsonlArchive) writeRecord(id servicechat.ID, ev servicechat.Event) ([]byte, error) {
	line, err := json.Marshal(eventRecordFromDomain(ev))
	if err != nil {
		return nil, err
	}
	line = append(line, '\n')

	f, err := os.OpenFile(
		a.store.eventsPath(id),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0o644,
	)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Write(line); err != nil {
		return nil, err
	}
	return line, nil
}

// replaceStream rewrites events.jsonl from a streaming source. Every event the
// source yields is encoded into a temporary file that replaces the archive
// only after the source completes, so a failed source leaves the original
// untouched.
func (a *jsonlArchive) replaceStream(
	ctx context.Context,
	id servicechat.ID,
	src eventSource,
) error {
	dir := a.store.chatDir(id)
	tmp := filepath.Join(dir, "events.jsonl.tmp")
	final := a.store.eventsPath(id)
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	if err := src(ctx, func(ev servicechat.Event) error {
		return enc.Encode(eventRecordFromDomain(ev))
	}); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Package chatmigration converts the frozen legacy chat files into Remote's
// authoritative SQLite store. Native harness state is deliberately untouched.
package chatmigration

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/stores/sqlitechat"
)

const maxRecordBytes = 16 * 1024 * 1024

type Options struct {
	BatchEvents int
	BatchBytes  int
	// Progress runs after a committed checkpoint. Cancellation is safe here.
	Progress func(sqlitechat.ImportProgress) error
}

type Result struct {
	Chats           int   `json:"chats"`
	Events          int64 `json:"events"`
	AlreadyComplete bool  `json:"alreadyComplete"`
}

type source struct {
	id            servicechat.ID
	meta          servicechat.Meta
	rawMeta       []byte
	path          string
	fingerprint   string
	missingEvents bool
}

// Run must execute while the backend is stopped. The store's exclusive lease
// also rejects concurrent new-version servers and importers.
func Run(ctx context.Context, root string, options Options) (result Result, err error) {
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

func importChat(ctx context.Context, store *sqlitechat.Store, src source, options Options) (sqlitechat.ImportProgress, error) {
	p, err := store.BeginImportChat(ctx, src.meta, src.rawMeta, src.fingerprint)
	if err != nil {
		return p, err
	}
	h := sha256.New()
	if len(p.HashState) > 0 {
		if err := h.(encoding.BinaryUnmarshaler).UnmarshalBinary(p.HashState); err != nil {
			return p, fmt.Errorf("invalid checksum checkpoint: %w", err)
		}
	}
	if p.Complete {
		return p, store.FinishImportChat(ctx, src.meta, src.rawMeta, p, hex.EncodeToString(h.Sum(nil)))
	}
	if !src.missingEvents {
		f, err := os.Open(src.path)
		if err != nil {
			return p, err
		}
		defer f.Close()
		if _, err := f.Seek(p.ByteOffset, io.SeekStart); err != nil {
			return p, err
		}
		reader := bufio.NewReaderSize(f, 64*1024)
		batch := make([]sqlitechat.ImportEvent, 0, options.BatchEvents)
		batchBytes := 0
		flush := func() error {
			var err error
			p.HashState, err = h.(encoding.BinaryMarshaler).MarshalBinary()
			if err != nil {
				return err
			}
			if err := store.ImportEventBatch(ctx, p, batch); err != nil {
				return err
			}
			batch = batch[:0]
			batchBytes = 0
			if options.Progress != nil {
				return options.Progress(p)
			}
			return nil
		}
		for {
			if err := ctx.Err(); err != nil {
				return p, err
			}
			line, readErr := readLine(reader)
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return p, readErr
			}
			p.ByteOffset += int64(len(line))
			raw := bytes.TrimSpace(line)
			if len(raw) > 0 {
				var event servicechat.Event
				if err := json.Unmarshal(raw, &event); err != nil {
					return p, fmt.Errorf("malformed event ending at byte %d: %w", p.ByteOffset, err)
				}
				if event.Type == "" {
					return p, fmt.Errorf("event ending at byte %d has no type", p.ByteOffset)
				}
				if event.Seq == 0 {
					event.Seq = p.LastSeq + 1
				}
				if event.Seq <= p.LastSeq {
					return p, fmt.Errorf("event sequence %d does not follow %d", event.Seq, p.LastSeq)
				}
				p.LastSeq = event.Seq
				p.EventCount++
				sqlitechat.HashImportEvent(h, event.Seq, raw)
				batch = append(batch, sqlitechat.ImportEvent{Event: event, Raw: raw})
				batchBytes += len(raw)
			}
			if len(batch) >= options.BatchEvents || batchBytes >= options.BatchBytes {
				if err := flush(); err != nil {
					return p, err
				}
			}
			if errors.Is(readErr, io.EOF) {
				if err := flush(); err != nil {
					return p, err
				}
				break
			}
		}
	}
	if err := store.FinishImportChat(ctx, src.meta, src.rawMeta, p, hex.EncodeToString(h.Sum(nil))); err != nil {
		return p, err
	}
	p.Complete = true
	return p, nil
}

func readLine(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		part, err := reader.ReadSlice('\n')
		if len(line)+len(part) > maxRecordBytes {
			return nil, fmt.Errorf("legacy event exceeds %d bytes", maxRecordBytes)
		}
		line = append(line, part...)
		if !errors.Is(err, bufio.ErrBufferFull) {
			return line, err
		}
	}
}

func discover(root string) ([]source, string, error) {
	entries, err := os.ReadDir(filepath.Join(root, "chats"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, hex.EncodeToString(sha256.New().Sum(nil)), nil
	}
	if err != nil {
		return nil, "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var sources []source
	manifest := sha256.New()
	for _, entry := range entries {
		id := servicechat.ID(entry.Name())
		if !entry.IsDir() || !servicechat.ValidID(id) {
			return nil, "", fmt.Errorf("unexpected entry in legacy chats directory: %s", entry.Name())
		}
		dir := filepath.Join(root, "chats", entry.Name())
		metaPath := filepath.Join(dir, "meta.json")
		stat, err := os.Lstat(metaPath)
		if err != nil {
			return nil, "", fmt.Errorf("chat %s metadata: %w", id, err)
		}
		if !stat.Mode().IsRegular() || stat.Size() > 4*1024*1024 {
			return nil, "", fmt.Errorf("chat %s metadata must be a regular file below 4 MiB", id)
		}
		rawMeta, err := os.ReadFile(metaPath)
		if err != nil {
			return nil, "", err
		}
		var meta servicechat.Meta
		if err := json.Unmarshal(rawMeta, &meta); err != nil {
			return nil, "", fmt.Errorf("chat %s metadata: %w", id, err)
		}
		if meta.ID != id {
			return nil, "", fmt.Errorf("chat directory %s does not match metadata ID %q", id, meta.ID)
		}
		meta.Provider = servicechat.NormalizeProvider(meta.Provider)
		meta.NormalizeSessions()
		src := source{id: id, meta: meta, rawMeta: rawMeta, path: filepath.Join(dir, "events.jsonl")}
		eventsStat, err := os.Lstat(src.path)
		if errors.Is(err, os.ErrNotExist) {
			// A newly created chat legitimately has only meta.json. A chat with
			// recorded activity must not silently become an empty conversation.
			if len(meta.Sessions) > 0 || meta.LastMessageAt > meta.CreatedAt {
				return nil, "", fmt.Errorf("chat %s is missing events.jsonl despite recorded activity", id)
			}
			src.missingEvents = true
		} else if err != nil {
			return nil, "", err
		} else if !eventsStat.Mode().IsRegular() {
			return nil, "", fmt.Errorf("chat %s events must be a regular file", id)
		}
		fingerprint := sha256.New()
		_, _ = fingerprint.Write(rawMeta)
		if src.missingEvents {
			_, _ = fingerprint.Write([]byte("\x00missing-events"))
		} else {
			_, _ = fmt.Fprintf(fingerprint, "\x00%d:%d:%d", eventsStat.Size(), eventsStat.ModTime().UnixNano(), eventsStat.Mode())
			if st, ok := eventsStat.Sys().(*syscall.Stat_t); ok {
				_, _ = fmt.Fprintf(fingerprint, ":%d:%d:%d:%d", st.Dev, st.Ino, st.Ctim.Sec, st.Ctim.Nsec)
			}
		}
		src.fingerprint = hex.EncodeToString(fingerprint.Sum(nil))
		_, _ = fmt.Fprintf(manifest, "%s:%s\n", id, src.fingerprint)
		sources = append(sources, src)
	}
	return sources, hex.EncodeToString(manifest.Sum(nil)), nil
}

func removeLegacyProjection(root string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(filepath.Join(root, "transcript-index.sqlite") + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

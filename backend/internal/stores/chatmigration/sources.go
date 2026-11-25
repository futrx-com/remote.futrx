package chatmigration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type source struct {
	id            servicechat.ID
	meta          servicechat.Meta
	rawMeta       []byte
	path          string
	fingerprint   string
	missingEvents bool
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

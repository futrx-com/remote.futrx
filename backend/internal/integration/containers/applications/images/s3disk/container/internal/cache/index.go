package cache

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// indexFile is the on-disk form of the cache, written on unmount so a remount
// keeps its warm blocks and can recover data that was never uploaded.
type indexFile struct {
	Version   int            `json:"version"`
	Identity  string         `json:"identity"`
	BlockSize int64          `json:"block_size"`
	Entries   []indexEntry   `json:"entries"`
	Written   time.Time      `json:"written"`
	Extra     map[string]any `json:"extra,omitempty"`
}

type indexEntry struct {
	Key     string `json:"key"`
	Size    int64  `json:"size"`
	ObjSize int64  `json:"obj_size"`
	ETag    string `json:"etag"`
	Mtime   int64  `json:"mtime"`
	Dirty   bool   `json:"dirty"`
	NewFile bool   `json:"new_file"`
	Blocks  string `json:"blocks"`
}

const indexVersion = 1

func (c *Cache) indexPath() string { return filepath.Join(c.dir, "index.json") }

func (c *Cache) saveIndex() error {
	c.mu.Lock()
	idx := indexFile{Version: indexVersion, Identity: c.opts.Identity, BlockSize: c.bs, Written: time.Now()}
	for _, e := range c.entries {
		e.mu.Lock()
		if !e.deleted {
			idx.Entries = append(idx.Entries, indexEntry{
				Key:     e.key,
				Size:    e.size,
				ObjSize: e.objSize,
				ETag:    e.etag,
				Mtime:   e.mtime.UnixNano(),
				Dirty:   e.dirty,
				NewFile: e.newFile,
				Blocks:  base64.StdEncoding.EncodeToString(e.blocks.bytes()),
			})
		}
		e.mu.Unlock()
	}
	c.mu.Unlock()

	tmp := c.indexPath() + ".tmp"
	f, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&idx); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return os.Rename(tmp, c.indexPath())
}

// loadIndex restores entries whose data files are still intact. Entries whose
// backing file is missing or the wrong size are silently dropped.
func (c *Cache) loadIndex() error {
	data, err := os.ReadFile(c.indexPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var idx indexFile
	if err := json.Unmarshal(data, &idx); err != nil {
		return err
	}
	if idx.Version != indexVersion {
		return fmt.Errorf("cache index version %d not supported", idx.Version)
	}
	if idx.BlockSize != c.bs {
		return fmt.Errorf("cache built with block size %d, now %d", idx.BlockSize, c.bs)
	}
	if idx.Identity != c.opts.Identity {
		return fmt.Errorf("cache belongs to %q, this mount is %q", idx.Identity, c.opts.Identity)
	}
	restored := 0
	for _, ie := range idx.Entries {
		p := c.pathFor(ie.Key)
		st, err := os.Stat(p)
		if err != nil || st.Size() != ie.Size {
			_ = os.Remove(p)
			continue
		}
		f, err := os.OpenFile(p, os.O_RDWR, 0600)
		if err != nil {
			continue
		}
		blockBytes, _ := base64.StdEncoding.DecodeString(ie.Blocks)
		e := &Entry{
			c:       c,
			key:     ie.Key,
			path:    p,
			file:    f,
			size:    ie.Size,
			objSize: ie.ObjSize,
			etag:    ie.ETag,
			mtime:   time.Unix(0, ie.Mtime),
			blocks:  bitmapFromBytes(blockBytes, int(nblocks(ie.ObjSize, c.bs))),
			dirty:   ie.Dirty,
			newFile: ie.NewFile,
			used:    time.Now(),
		}
		if ie.Dirty {
			e.dirtySince = time.Now()
		}
		c.entries[ie.Key] = e
		c.bytes.Add(e.diskBytesLocked()) // freshly built, not yet shared
		restored++
	}
	if restored > 0 {
		c.opts.Log("cache: restored %d entries (%s) from %s", restored, humanBytes(c.bytes.Load()), c.dir)
	}
	return nil
}

// Recover uploads data that was dirty when the previous mount ended, which is
// how a crashed or killed mount avoids losing writes.
func (c *Cache) Recover(ctx context.Context, log func(string, ...any)) (int, error) {
	dirty := c.DirtyKeys()
	if len(dirty) == 0 {
		return 0, nil
	}
	log("recovering %d unsaved file(s) from the previous mount", len(dirty))
	n := 0
	for _, k := range dirty {
		e := c.Lookup(k)
		if e == nil {
			continue
		}
		if err := e.Flush(ctx); err != nil {
			return n, fmt.Errorf("recovering %s: %w", k, err)
		}
		log("recovered %s", k)
		n++
	}
	return n, nil
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

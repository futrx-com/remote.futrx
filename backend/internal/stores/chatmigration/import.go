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

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/stores/sqlitechat"
)

const maxRecordBytes = 16 * 1024 * 1024

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

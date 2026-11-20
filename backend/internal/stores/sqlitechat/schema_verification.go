package sqlitechat

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Include every original canonical column, including exact payload/raw bytes,
// metadata, ancestry, and import checkpoints. Length/type framing distinguishes
// NULL, empty data, and concatenation boundaries without decoding JSON.
func canonicalSchemaDigest(ctx context.Context, q sqlQuerier) (string, error) {
	h := sha256.New()
	queries := []string{
		"SELECT id,project_id,active_branch_id,title,last_message_at,meta_json FROM chats ORDER BY id",
		"SELECT id,chat_id,parent_branch_id,fork_seq,last_seq,last_turn_ordinal FROM branches ORDER BY id",
		"SELECT branch_id,seq,t,event_type,turn_id,payload_json,raw_json FROM events ORDER BY branch_id,seq",
		"SELECT id,status,source_manifest,started_at,completed_at FROM conversation_migration ORDER BY id",
		"SELECT chat_id,branch_id,fingerprint,byte_offset,event_count,last_seq,hash_state,status FROM conversation_imports ORDER BY chat_id",
	}
	for _, query := range queries {
		h.Write([]byte(query))
		rows, err := q.QueryContext(ctx, query)
		if err != nil {
			return "", err
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			return "", err
		}
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		for rows.Next() {
			if err = rows.Scan(pointers...); err != nil {
				rows.Close()
				return "", err
			}
			for _, value := range values {
				var data []byte
				switch v := value.(type) {
				case nil:
					data = []byte{0}
				case []byte:
					data = append([]byte{1}, v...)
				case string:
					data = append([]byte{2}, v...)
				case int64:
					data = make([]byte, 9)
					data[0] = 3
					binary.BigEndian.PutUint64(data[1:], uint64(v))
				default:
					rows.Close()
					return "", fmt.Errorf("unexpected canonical field type %T", v)
				}
				var size [8]byte
				binary.BigEndian.PutUint64(size[:], uint64(len(data)))
				h.Write(size[:])
				h.Write(data)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func verifySchemaDatabase(ctx context.Context, q sqlQuerier) error {
	for _, check := range []string{"integrity_check", "foreign_key_check"} {
		rows, err := q.QueryContext(ctx, "PRAGMA "+check)
		if err != nil {
			return err
		}
		if check == "integrity_check" {
			for rows.Next() {
				var result string
				if err = rows.Scan(&result); err != nil {
					rows.Close()
					return err
				}
				if strings.TrimSpace(result) != "ok" {
					rows.Close()
					return fmt.Errorf("SQLite integrity check: %s", result)
				}
			}
		} else if rows.Next() {
			rows.Close()
			return errors.New("SQLite foreign key check failed")
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

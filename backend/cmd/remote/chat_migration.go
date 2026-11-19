package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/stores/chatmigration"
	"github.com/futrx-com/remote.futrx.com/internal/stores/sqlitechat"
)

func runChatMigration(ctx context.Context, root string, out io.Writer) error {
	var lastChat string
	var lastReport time.Time
	result, err := chatmigration.Run(ctx, root, chatmigration.Options{
		Progress: func(p sqlitechat.ImportProgress) error {
			if lastChat == string(p.ChatID) && time.Since(lastReport) < time.Second {
				return nil
			}
			lastChat = string(p.ChatID)
			lastReport = time.Now()
			// Only identifiers and counts: chat contents never enter upgrade logs.
			_, err := fmt.Fprintf(out, "Imported chat %s: %d events, byte %d\n", p.ChatID, p.EventCount, p.ByteOffset)
			return err
		},
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}

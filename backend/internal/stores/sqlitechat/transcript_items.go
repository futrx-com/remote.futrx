package sqlitechat

import (
	"strconv"
	"unicode/utf8"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type transcriptItemMode uint8

const (
	transcriptItemSingle transcriptItemMode = iota
	transcriptItemReplace
	transcriptItemAppend
)

func transcriptItemIdentity(event servicechat.Event, ordinal int64) (string, transcriptItemMode, bool) {
	switch event.Type {
	case "provider_event", "usage_update", "session", "sync", "system", "permission_request":
		return "", transcriptItemSingle, false
	case "collaboration":
		if event.Name == "wait" {
			return "", transcriptItemSingle, false
		}
		if event.ID != "" {
			return "collaboration:" + event.ID, transcriptItemReplace, true
		}
	case "turn_status":
		return "turn-status", transcriptItemReplace, true
	case "tool_use_start", "tool_use_end":
		if event.ID != "" {
			return "tool:" + event.ID, transcriptItemAppend, true
		}
	case "interaction_request", "interaction_resolved":
		if event.ID != "" {
			return "interaction:" + event.ID, transcriptItemAppend, true
		}
	}
	return "event:" + strconv.FormatInt(ordinal, 10), transcriptItemSingle, true
}

func utf8Prefix(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}

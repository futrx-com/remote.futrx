package filechat

import (
	"context"
	"fmt"
	"strings"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// Backend selects the storage engine that owns a chat's event stream.
type Backend string

const (
	// BackendJSONL keeps each chat in its own append-only events.jsonl file.
	BackendJSONL Backend = "jsonl"
	// BackendSQLite keeps every chat in the shared chats.sqlite database and
	// mirrors appended events back to events.jsonl.
	BackendSQLite Backend = "sqlite"
)

// ParseBackend resolves a configured backend name. An empty value selects the
// default backend, the same one CHAT_STORE falls back to when it is unset.
func ParseBackend(raw string) (Backend, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return Backend(configconstants.DefaultChatStoreBackend), nil
	case string(BackendJSONL):
		return BackendJSONL, nil
	case string(BackendSQLite):
		return BackendSQLite, nil
	default:
		return "", fmt.Errorf("unknown chat store backend %q (want %q or %q)",
			raw, BackendJSONL, BackendSQLite)
	}
}

// eventSource streams a chat's events in storage order without materializing
// the whole conversation. A non-nil error returned by yield stops the walk and
// is handed back to the caller.
type eventSource func(ctx context.Context, yield func(servicechat.Event) error) error

// eventLog owns durable event storage for one chat. Store keeps meta.json,
// per-chat locking, and the derived transcript index; the log only reads and
// writes the event stream itself. Every method is called with the chat lock
// already held by Store.
type eventLog interface {
	// Create initializes an empty stream for id.
	Create(ctx context.Context, id servicechat.ID) error
	// Remove discards durable event state for id. Removing the chat directory
	// itself remains the Store's responsibility.
	Remove(ctx context.Context, id servicechat.ID) error
	// Append assigns the next sequence number and durably stores the event.
	Append(ctx context.Context, id servicechat.ID, ev servicechat.Event) (servicechat.Event, error)
	// Replace rewrites the whole stream with events, preserving each event's
	// sequence number. It is the durability half of a rewind.
	Replace(ctx context.Context, id servicechat.ID, events []servicechat.Event) error
	// TruncateBefore keeps only the events whose timestamp is below beforeT
	// and reports the highest kept timestamp, or 0 when none are kept. It
	// streams in bounded batches rather than materializing the stream, so a
	// rewind of a large conversation stays flat in memory.
	TruncateBefore(ctx context.Context, id servicechat.ID, beforeT int64) (int64, error)
	ReadAll(ctx context.Context, id servicechat.ID) ([]servicechat.Event, error)
	// Scan visits events in storage order without materializing the stream.
	Scan(ctx context.Context, id servicechat.ID, visit func(servicechat.Event) bool) error
	ReadPage(ctx context.Context, id servicechat.ID, beforeSeq int64, limit int) (servicechat.EventPage, error)
	ReadAfter(ctx context.Context, id servicechat.ID, afterSeq int64) ([]servicechat.Event, error)
	// LastSeq reports the highest stored sequence number for id.
	LastSeq(ctx context.Context, id servicechat.ID) (int64, error)
	// CopyEvents copies from's stored stream onto to, assigning fresh
	// sequence numbers, in batches bounded by memory. It reports how many
	// events were written and the last one written. The caller holds both
	// chat locks.
	CopyEvents(
		ctx context.Context,
		from servicechat.ID,
		to servicechat.ID,
	) (int, servicechat.Event, error)
	// Prepare makes the log ready to serve reads for id, importing archived
	// history when the backend keeps a second copy.
	Prepare(ctx context.Context, id servicechat.ID) error
	Close() error
}

var (
	_ eventLog = (*jsonlLog)(nil)
	_ eventLog = (*sqliteLog)(nil)
)

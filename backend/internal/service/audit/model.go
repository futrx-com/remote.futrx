// Package audit records who did what on this server.
//
// The platform has exactly one process writing DATA_DIR, so the audit trail
// is an append-only JSONL file per calendar month. Entries are written on the
// caller's goroutine but never surface a failure to it: an unrecordable event
// is logged and dropped rather than turned into a 500 for the user.
package audit

import (
	"context"
	"strings"
	"time"
)

// Actor is the authenticated principal behind a recorded action. All three
// fields are empty for actions taken by the server itself (schedulers,
// janitors, reconciliation).
type Actor struct {
	Email   string `json:"email,omitempty"`
	Sub     string `json:"sub,omitempty"`
	IsAdmin bool   `json:"isAdmin,omitempty"`
}

// Empty reports whether no principal could be resolved.
func (a Actor) Empty() bool {
	return a.Email == "" && a.Sub == ""
}

// Target identifies the thing acted upon. Type is a stable noun ("project",
// "user", "secret"); ID is what the read API filters on; Name is a
// human-readable label captured at write time so the log stays readable after
// the target is deleted or renamed.
type Target struct {
	Type string `json:"type,omitempty"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// Meta carries action-specific detail. Never put secret values in here — the
// audit log records that a secret was read, not what it contained.
type Meta map[string]any

// Entry is one line of the audit log.
type Entry struct {
	At        time.Time `json:"at"`
	Actor     Actor     `json:"actor"`
	Action    string    `json:"action"`
	Target    Target    `json:"target,omitempty"`
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"userAgent,omitempty"`
	Meta      Meta      `json:"meta,omitempty"`
	OK        bool      `json:"ok"`
	Error     string    `json:"error,omitempty"`
}

// Recorder is the write side instrumented code depends on. Keeping it an
// interface lets services take an audit sink without importing the store.
type Recorder interface {
	Record(ctx context.Context, entry Entry)
}

// Nop is a Recorder that drops everything. Constructors substitute it for a
// nil recorder so instrumented code never needs a nil check.
type Nop struct{}

func (Nop) Record(context.Context, Entry) {}

// RecorderOrNop normalizes a possibly-nil recorder.
func RecorderOrNop(recorder Recorder) Recorder {
	if recorder == nil {
		return Nop{}
	}
	return recorder
}

// Success is the common shorthand for "this happened and it worked".
func Success(action string, target Target, meta Meta) Entry {
	return Entry{Action: action, Target: target, Meta: meta, OK: true}
}

// Result folds an error into an entry, so instrumented call sites can record
// the attempt and its outcome in one statement.
func Result(action string, target Target, meta Meta, err error) Entry {
	entry := Entry{Action: action, Target: target, Meta: meta, OK: err == nil}
	if err != nil {
		entry.Error = err.Error()
	}
	return entry
}

// NormalizeActorEmail matches how the rest of the platform keys identities.
func NormalizeActorEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

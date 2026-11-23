package lifecycle

import (
	"encoding/json"
	"fmt"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

const (
	pushPublisher = "push"
	pushCompleted = "completed"
	pushVersion   = 1
)

// PushOutcome summarizes one finished chat attachment push request. Stored
// includes files already present at the destination; issues includes per-file
// errors, including uploads awaiting asynchronous writeback.
type PushOutcome struct {
	Directory string `json:"directory"`
	Requested int    `json:"requested"`
	Stored    int    `json:"stored"`
	Removed   int    `json:"removed"`
	Issues    int    `json:"issues"`
}

// PushEvents is the request API's typed trigger for push publications.
type PushEvents interface {
	Completed(PushOutcome) error
}

// Pushes publishes the manifest-declared push event through Remote's runtime.
type Pushes struct {
	events applications.EventEmitter
}

var _ PushEvents = (*Pushes)(nil)

func NewPushes(events applications.EventEmitter) *Pushes {
	return &Pushes{events: events}
}

func (p *Pushes) Completed(outcome PushOutcome) error {
	if p.events == nil {
		return fmt.Errorf("application event runtime is unavailable")
	}
	payload, err := json.Marshal(outcome)
	if err != nil {
		return fmt.Errorf("encode push event: %w", err)
	}
	return p.events.Emit(applications.Publication{
		Publisher: pushPublisher,
		Event:     pushCompleted,
		Version:   pushVersion,
		Payload:   payload,
	})
}

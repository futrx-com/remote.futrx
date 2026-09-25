// Package lifecycle owns Hello Remote's business-level application events. It
// is a sibling of api because event identity and payloads are lifecycle
// concerns, while Remote core owns the event runtime and actual publication.
package lifecycle

import (
	"encoding/json"
	"fmt"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

const (
	greetingsPublisher = "greetings"
	greetedEvent       = "greeted"
	greetedVersion     = 1
)

// GreetingEvents is the API-facing contract owned by the greetings publisher.
// Consumers depend on the business event they can trigger, not its concrete
// implementation or Remote's runtime transport.
type GreetingEvents interface {
	Greeted(int) error
}

// Greetings exposes typed triggers for the manifest-declared greetings
// publisher. It neither registers nor implements a publisher: Remote creates
// and owns the emitter from application.json, and this layer only submits
// business events to it.
type Greetings struct {
	events applications.EventEmitter
}

var _ GreetingEvents = (*Greetings)(nil)

func NewGreetings(events applications.EventEmitter) *Greetings {
	return &Greetings{events: events}
}

func (g *Greetings) Greeted(visits int) error {
	if g.events == nil {
		return fmt.Errorf("application event runtime is unavailable")
	}
	payload, err := json.Marshal(map[string]int{"visits": visits})
	if err != nil {
		return fmt.Errorf("encode greeted event: %w", err)
	}
	return g.events.Emit(applications.Publication{
		Publisher: greetingsPublisher,
		Event:     greetedEvent,
		Version:   greetedVersion,
		Payload:   payload,
	})
}

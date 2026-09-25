package applications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	applicationapi "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

const maxEventPayloadBytes = applicationapi.MaxEventPayloadBytes

// instancePublisher is the core-owned emitter bound into one child process. It
// authorizes against that process's validated manifest and stamps identity the
// child cannot forge before an event reaches the shared bus.
type instancePublisher struct {
	instance applicationapi.Instance
	events   EventSink
}

var _ applicationapi.EventEmitter = (*instancePublisher)(nil)

func newInstancePublisher(instance applicationapi.Instance, events EventSink) *instancePublisher {
	return &instancePublisher{instance: instance, events: events}
}

func (p *instancePublisher) Emit(publication applicationapi.Publication) error {
	if p.events == nil {
		return fmt.Errorf("event bus unavailable")
	}
	if !declaresPublication(p.instance.Publishers, publication) {
		return fmt.Errorf(
			"%s.%s version %d is not declared by application %s",
			publication.Publisher,
			publication.Event,
			publication.Version,
			p.instance.ApplicationID,
		)
	}
	payload, err := validEventPayload(publication.Payload)
	if err != nil {
		return fmt.Errorf("%s.%s: %w", publication.Publisher, publication.Event, err)
	}

	p.events.Publish(context.Background(), applicationapi.Event{
		Source: applicationapi.EventSource{
			ApplicationID: p.instance.ApplicationID,
			InstanceID:    p.instance.ID,
			Scope:         p.instance.Scope,
			ProjectID:     p.instance.ProjectID,
			Publisher:     canonicalPublisher(p.instance.ApplicationID, publication.Publisher),
		},
		Name:    publication.Event,
		Version: publication.Version,
		Payload: payload,
	})
	return nil
}

func declaresPublication(
	publishers []applicationapi.PublisherDeclaration,
	publication applicationapi.Publication,
) bool {
	for _, publisher := range publishers {
		if publisher.Name != publication.Publisher {
			continue
		}
		for _, event := range publisher.Events {
			if event.Name == publication.Event && event.Version == publication.Version {
				return true
			}
		}
		return false
	}
	return false
}

func validEventPayload(payload json.RawMessage) (json.RawMessage, error) {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	if len(payload) > maxEventPayloadBytes {
		return nil, fmt.Errorf("payload exceeds %d bytes", maxEventPayloadBytes)
	}
	if !json.Valid(payload) {
		return nil, fmt.Errorf("payload must be valid JSON")
	}
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("payload must be a JSON object")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		return nil, fmt.Errorf("payload must be valid JSON: %w", err)
	}
	if object == nil {
		return nil, fmt.Errorf("payload must be a JSON object")
	}
	return append(json.RawMessage(nil), payload...), nil
}

func canonicalPublisher(applicationID, publisher string) string {
	return "applications." + applicationID + "." + publisher
}

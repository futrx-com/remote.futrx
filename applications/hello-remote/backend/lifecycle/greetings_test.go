package lifecycle

import (
	"encoding/json"
	"testing"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type recordingEmitter struct {
	publication applications.Publication
}

func (emitter *recordingEmitter) Emit(publication applications.Publication) error {
	publication.Payload = append(json.RawMessage(nil), publication.Payload...)
	emitter.publication = publication
	return nil
}

func TestGreetedUsesTheDeclaredEventContract(t *testing.T) {
	emitter := &recordingEmitter{}
	greetings := NewGreetings(emitter)
	if err := greetings.Greeted(3); err != nil {
		t.Fatalf("emit greeting: %v", err)
	}

	publication := emitter.publication
	if publication.Publisher != "greetings" || publication.Event != "greeted" ||
		publication.Version != 1 {
		t.Fatalf("publication identity = %+v", publication)
	}
	var payload map[string]int
	if err := json.Unmarshal(publication.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["visits"] != 3 {
		t.Fatalf("payload = %v, want visits 3", payload)
	}
}

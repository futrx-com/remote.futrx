package lifecycle

import (
	"encoding/json"
	"testing"
)

func TestContainerInspectedUsesTheDeclaredEventContract(t *testing.T) {
	emitter := &recordingEmitter{}
	inspections := NewInspections(emitter)
	if err := inspections.ContainerInspected("futrx-app-1", "hello"); err != nil {
		t.Fatalf("emit inspection: %v", err)
	}

	publication := emitter.publication
	if publication.Publisher != "inspections" ||
		publication.Event != "container-inspected" || publication.Version != 1 {
		t.Fatalf("publication identity = %+v", publication)
	}
	var payload map[string]string
	if err := json.Unmarshal(publication.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["container"] != "futrx-app-1" || payload["hostname"] != "hello" {
		t.Fatalf("payload = %v", payload)
	}
}

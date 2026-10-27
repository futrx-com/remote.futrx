package lifecycle

import (
	"encoding/json"
	"fmt"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

const (
	inspectionsPublisher      = "inspections"
	containerInspectedEvent   = "container-inspected"
	containerInspectedVersion = 1
)

// InspectionEvents is the API-facing contract owned by the inspections
// publisher.
type InspectionEvents interface {
	ContainerInspected(container, hostname string) error
}

// Inspections exposes typed triggers for the independently declared
// inspections publisher.
type Inspections struct {
	events applications.EventEmitter
}

var _ InspectionEvents = (*Inspections)(nil)

func NewInspections(events applications.EventEmitter) *Inspections {
	return &Inspections{events: events}
}

func (i *Inspections) ContainerInspected(container, hostname string) error {
	if i.events == nil {
		return fmt.Errorf("application event runtime is unavailable")
	}
	payload, err := json.Marshal(map[string]string{
		"container": container,
		"hostname":  hostname,
	})
	if err != nil {
		return fmt.Errorf("encode container-inspected event: %w", err)
	}
	return i.events.Emit(applications.Publication{
		Publisher: inspectionsPublisher,
		Event:     containerInspectedEvent,
		Version:   containerInspectedVersion,
		Payload:   payload,
	})
}

package lifecycle

import (
	"encoding/json"
	"errors"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
	"testing"
)

type emitter struct {
	publication applications.Publication
	err         error
}

func (e *emitter) Emit(p applications.Publication) error { e.publication = p; return e.err }
func TestDuePublicationContractAndFailure(t *testing.T) {
	e := &emitter{}
	tasks := NewTasks(e)
	if err := tasks.Due("task", "run"); err != nil {
		t.Fatal(err)
	}
	p := e.publication
	var body map[string]string
	_ = json.Unmarshal(p.Payload, &body)
	if p.Publisher != "tasks" || p.Event != "due" || p.Version != 1 || body["taskId"] != "task" || body["runId"] != "run" {
		t.Fatalf("publication: %+v", p)
	}
	e.err = errors.New("failed")
	if !errors.Is(tasks.Due("task", "run"), e.err) {
		t.Fatal("publication failure swallowed")
	}
}

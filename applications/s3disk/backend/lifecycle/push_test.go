package lifecycle

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type recordingEmitter struct {
	publications []applications.Publication
}

func (e *recordingEmitter) Emit(publication applications.Publication) error {
	publication.Payload = append(json.RawMessage(nil), publication.Payload...)
	e.publications = append(e.publications, publication)
	return nil
}

func TestPushCompletedUsesDeclaredEventAndSummary(t *testing.T) {
	emitter := &recordingEmitter{}
	pushes := NewPushes(emitter)
	want := PushOutcome{Directory: "/workspace/s3/uploads", Requested: 3, Stored: 2, Removed: 1, Issues: 1}
	if err := pushes.Completed(want); err != nil {
		t.Fatal(err)
	}
	if len(emitter.publications) != 1 {
		t.Fatalf("publications = %d, want 1", len(emitter.publications))
	}
	publication := emitter.publications[0]
	if publication.Publisher != "push" || publication.Event != "completed" || publication.Version != 1 {
		t.Fatalf("publication identity = %+v", publication)
	}
	var got PushOutcome
	if err := json.Unmarshal(publication.Payload, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("payload = %+v, want %+v", got, want)
	}
}

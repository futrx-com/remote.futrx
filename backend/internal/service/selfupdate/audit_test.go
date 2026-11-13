package selfupdate

import (
	"context"
	"errors"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/service/audit"
)

type updateAuditRecorder struct{ entries []audit.Entry }

func (r *updateAuditRecorder) Record(_ context.Context, entry audit.Entry) {
	r.entries = append(r.entries, entry)
}

func TestApplyAuditsResolvedTagAlongsideLifecycle(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "started", true: "failed"}[fail], func(t *testing.T) {
			host := &fakeHost{tags: []string{"0.30.1"}, pid: 123, alive: true}
			if fail {
				host.startErr = errors.New("start failed")
			}
			recorder := &updateAuditRecorder{}
			lifecycle := &recordingUpdateLifecyclePublisher{}
			service := New("0.30.0", t.TempDir(), t.TempDir(), host, lifecycle, WithAudit(recorder))
			_, err := service.Apply(context.Background(), "admin@example.com", "")
			if (err != nil) != fail {
				t.Fatalf("Apply error = %v", err)
			}
			if len(lifecycle.events) == 0 || lifecycle.events[0].state != "started" {
				t.Fatalf("update lifecycle lost: %+v", lifecycle.events)
			}
			if len(recorder.entries) != 1 {
				t.Fatalf("audit count = %d", len(recorder.entries))
			}
			entry := recorder.entries[0]
			if entry.OK == fail || entry.Meta["tag"] != "0.30.1" || entry.Actor.Email != "admin@example.com" {
				t.Fatalf("update audit = %+v", entry)
			}
		})
	}
}

package prompt

import (
	"context"
	"encoding/json"
	"github.com/futrx-com/remote.futrx.com/internal/service/audit"
	"reflect"
	"strings"
	"testing"
)

type runAuditRecorder struct{ entries []audit.Entry }

func (r *runAuditRecorder) Record(_ context.Context, e audit.Entry) { r.entries = append(r.entries, e) }
func TestAuditRunPreservesOwnerAndCorrelationWithoutPromptText(t *testing.T) {
	for _, fail := range []bool{false, true} {
		recorder := &runAuditRecorder{}
		service, _, meta := newUsagePromptService(t, &usageProvider{fail: fail}, &recordingLedger{}, WithAudit(recorder))
		handle, err := service.Start(StartInput{ChatID: meta.ID, Prompt: "private prompt should not be audited", Actor: Actor{Email: "owner@example.com"}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		for range handle.Done {
		}
		actions := make([]string, 0, len(recorder.entries))
		for _, entry := range recorder.entries {
			actions = append(actions, entry.Action)
		}
		if want := []string{"agent.run.start", "prompt.submit", "agent.account.use", "agent.run.complete"}; !reflect.DeepEqual(actions, want) {
			t.Fatalf("audit actions = %v, want %v", actions, want)
		}
		var runID any
		for _, e := range recorder.entries {
			if e.Actor.Email != "owner@example.com" {
				t.Fatal(e)
			}
			if runID == nil {
				runID = e.Meta["runId"]
			}
			if e.Meta["runId"] != runID || runID == "" {
				t.Fatal(e)
			}
			data, _ := json.Marshal(e)
			if strings.Contains(string(data), "private prompt") {
				t.Fatal(string(data))
			}
		}
		last := recorder.entries[len(recorder.entries)-1]
		if last.Action != "agent.run.complete" || last.OK == fail {
			t.Fatal(last)
		}
		wantError := ""
		if fail {
			wantError = "agent run failed"
		}
		if last.Error != wantError {
			t.Fatalf("completion error = %q, want %q", last.Error, wantError)
		}
	}
}

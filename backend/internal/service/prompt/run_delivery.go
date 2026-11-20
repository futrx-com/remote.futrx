package prompt

import (
	"context"
	"errors"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

// runDelivery separates protocol acceptance from UI completion. A completed
// text stream without acceptance evidence cannot make a native session resumable.
type runDelivery struct {
	store            chat.ConversationRuntime
	run              chat.RunReference
	nativeID         string
	nativeCheckpoint string
	completed        bool
	failed           bool
	interrupted      bool
	acceptanceErr    error
	accepted         bool
	inputObserved    bool
}

func (d *runDelivery) observe(ctx context.Context, event harness.Event) {
	if event.SessionID != "" {
		d.nativeID = event.SessionID
	}
	switch event.Type {
	case harness.EventInputAccepted:
		d.inputObserved = true
		if d.accepted || d.acceptanceErr != nil {
			return
		}
		evidence := chat.InputAcceptance{NativeSessionID: d.nativeID, Evidence: event.Status}
		d.acceptanceErr = d.store.AcceptRunInput(ctx, d.run, evidence)
		d.accepted = d.acceptanceErr == nil
	case harness.EventRunCompleted:
		d.completed = true
		d.nativeCheckpoint = event.NativeCheckpoint
	case harness.EventRunFailed:
		d.failed = true
	case harness.EventRunInterrupted:
		d.interrupted = true
	}
}

func (d *runDelivery) finish(ctx context.Context, cancellation, errorFromRun error) error {
	err := errors.Join(errorFromRun, d.acceptanceErr)
	// Recovery errors are replay-safe only before input reached the harness.
	// Never retry a provider that reports one after accepting the current turn.
	if d.inputObserved && (errors.Is(err, harness.ErrSessionNotFound) || errors.Is(err, harness.ErrNativeCheckpointUnavailable)) {
		err = errors.New(err.Error())
	}
	status := "uncertain"
	switch {
	case cancellation != nil || d.interrupted:
		status = "interrupted"
	case err != nil || d.failed:
		status = "failed"
	case d.completed:
		status = "completed"
	}
	outcome := chat.RunOutcome{Status: status, NativeSessionID: d.nativeID, NativeCheckpoint: d.nativeCheckpoint}
	return errors.Join(err, d.store.FinishRun(ctx, d.run, outcome))
}

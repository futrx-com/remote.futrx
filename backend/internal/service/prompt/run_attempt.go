package prompt

import (
	"context"
	"errors"

	"github.com/futrx-com/remote.futrx.com/internal/harness"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// providerAttempt contains the inputs for one provider invocation. The caller
// passes the prepared history explicitly again when a missing session is retried.
type providerAttempt struct {
	provider harness.Provider
	input    StartInput
	meta     ChatMeta
	ledger   ledgerRun
	request  harness.RunRequest
}

func (rnr *Service) runAttempt(
	ctx context.Context,
	attempt providerAttempt,
	prepared servicechat.PreparedRun,
	emit func(ChatEvent),
) (runErr error) {
	id, meta := attempt.input.ChatID, attempt.meta
	request := attempt.request
	providerID := request.Provider
	var tracking *runDelivery
	if rnr.conversationRuntime != nil {
		tracking = &runDelivery{store: rnr.conversationRuntime, run: prepared.RunReference, nativeID: request.ResumeID}
		defer func() {
			if persistenceErr := rnr.hub.PersistenceError(id); persistenceErr != nil {
				runErr = errors.Join(runErr, persistenceErr)
			}
			runErr = tracking.finish(context.WithoutCancel(ctx), ctx.Err(), runErr)
		}()
	}
	if persistenceErr := rnr.hub.PersistenceError(id); persistenceErr != nil {
		return persistenceErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if rnr.accountAccess != nil {
		if _, err := rnr.accountAccess.Resolve(ctx, providerID, meta.AccountID); err != nil {
			return err
		}
	}
	rnr.recordAccountUse(ctx, attempt.input, meta, providerID, attempt.ledger.runID)
	relay := runEventRelay{
		service: rnr, ctx: ctx, chatID: id, providerID: providerID,
		ledger: attempt.ledger, emit: emit,
	}
	if rnr.conversationRuntime != nil {
		adapter, ok := attempt.provider.(harness.HistoryInputAdapter)
		if !ok {
			return errors.New(string(providerID) + " does not implement conversation history delivery")
		}
		var method string
		var err error
		request.Prompt, method, err = adapter.HistoryInput(prepared.History, request.Prompt)
		if err != nil {
			return err
		}
		if err = rnr.conversationRuntime.RecordRunInput(ctx, prepared.RunReference, method); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	var completion *harness.Event
	request.TrackInputAcceptance = tracking != nil
	runErr = attempt.provider.Run(runCtx, request, func(event harness.Event) {
		event = withDefaultProvider(event, providerID)
		if event.AccountID == "" {
			event.AccountID = meta.AccountID
		}
		if tracking != nil {
			tracking.observe(context.WithoutCancel(ctx), event)
			if tracking.acceptanceErr != nil {
				cancelRun()
			}
		}
		if meta.SetupRepair && event.Type == harness.EventRunCompleted {
			completion = &event
			return
		}
		relay.forward(event)
	})
	if runErr == nil && meta.SetupRepair {
		verifier, ok := rnr.projects.(interface {
			VerifySetup(context.Context, serviceproject.ID) (serviceproject.Meta, error)
		})
		if !ok {
			runErr = errors.New("project setup verifier unavailable")
		} else {
			_, runErr = verifier.VerifySetup(ctx, serviceproject.ID(meta.ProjectID))
		}
	}
	if completion != nil {
		if runErr != nil {
			// The provider spent these tokens even when setup still fails.
			// Keep its completion for accounting, with a failed turn status
			// and a platform result instead of the agent's success summary.
			completion.Status = "failed"
			relay.emit = func(event ChatEvent) {
				if event.Type == "complete" {
					event.NotificationSummary = "Automatic setup repair did not pass verification. Open the repair chat for details."
				}
				emit(event)
			}
		}
		relay.forward(*completion)
	}
	relay.finish()
	return runErr
}

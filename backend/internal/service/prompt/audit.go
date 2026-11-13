package prompt

import (
	"context"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/service/audit"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// WithAudit records the start and cancellation of every agent run.
func WithAudit(recorder audit.Recorder) Option {
	return func(service *Service) {
		service.audit = audit.RecorderOrNop(recorder)
	}
}

// recordRun writes one agent-run line. The provider and project come from the
// chat record, so a run is attributable even though the audit log never sees
// the prompt text.
func (rnr *Service) recordRun(ctx context.Context, action string, input StartInput, err error) {
	if rnr.audit == nil {
		return
	}
	meta := audit.Meta{"chatId": string(input.ChatID)}
	if chat, chatErr := rnr.store.Get(ctx, input.ChatID); chatErr == nil {
		meta["provider"] = string(providerIDFromChatProvider(chat.Provider))
		if chat.ProjectID != "" {
			meta["projectId"] = string(chat.ProjectID)
		}
	}
	if input.ApplicationInstanceID != "" {
		meta["applicationId"] = input.ApplicationID
		meta["applicationInstanceId"] = input.ApplicationInstanceID
		meta["applicationRequestId"] = input.ApplicationRequestID
	}
	entry := audit.Result(
		action,
		audit.Target{Type: audit.TargetChat, ID: string(input.ChatID)},
		meta,
		err,
	)
	if input.Actor.Email != "" {
		entry.Actor = audit.Actor{
			Email:   audit.NormalizeActorEmail(input.Actor.Email),
			IsAdmin: input.Actor.IsAdmin,
		}
	}
	rnr.audit.Record(ctx, entry)
}

// recordStartedRun captures the selected account and application correlation
// once. Completion uses this same entry even if execution changes chat metadata.
func (rnr *Service) recordStartedRun(ctx context.Context, input StartInput, meta ChatMeta, runID string) audit.Entry {
	detail := audit.Meta{"runId": runID, "provider": string(meta.Provider), "accountId": meta.AccountID, "projectId": string(meta.ProjectID), "applicationId": input.ApplicationID, "applicationInstanceId": input.ApplicationInstanceID, "applicationRequestId": input.ApplicationRequestID}
	entry := audit.Success(audit.ActionAgentRunStart, audit.Target{Type: audit.TargetChat, ID: string(input.ChatID)}, detail)
	entry.Actor = audit.Actor{Email: input.Actor.Email, IsAdmin: input.Actor.IsAdmin}
	rnr.audit.Record(ctx, entry)
	submitted := entry
	submitted.Action = audit.ActionPromptSubmit
	rnr.audit.Record(ctx, submitted)
	return entry
}

func (rnr *Service) recordCompletedRun(ctx context.Context, started audit.Entry, err error) {
	completed := started
	completed.Action = audit.ActionAgentRunComplete
	completed.OK = err == nil
	if err != nil {
		completed.Error = "agent run failed"
	}
	rnr.audit.Record(ctx, completed)
}

func (rnr *Service) recordAccountUse(ctx context.Context, input StartInput, meta ChatMeta, providerID agent.ProviderID, runID string) {
	if rnr.audit != nil {
		entry := audit.Success(audit.ActionAgentAccountUse, audit.Target{Type: audit.TargetChat, ID: string(input.ChatID)}, audit.Meta{"runId": runID, "projectId": string(meta.ProjectID), "provider": string(providerID), "accountId": meta.AccountID, "applicationInstanceId": input.ApplicationInstanceID, "applicationRequestId": input.ApplicationRequestID})
		entry.Actor = audit.Actor{Email: input.Actor.Email, IsAdmin: input.Actor.IsAdmin}
		rnr.audit.Record(ctx, entry)
	}
}

func (rnr *Service) recordRunCancellation(ctx context.Context, id servicechat.ID, canceled bool) {
	if rnr.audit != nil {
		rnr.audit.Record(ctx, audit.Result(
			audit.ActionAgentRunCancel,
			audit.Target{Type: audit.TargetChat, ID: string(id)},
			audit.Meta{"canceled": canceled},
			nil,
		))
	}
}

import { DecisionButton, RequestDetails } from "./InteractionControls";
import type { InteractionFormProps } from "./types";

export function ApprovalInteractionForm({
  input,
  disabled,
  supportsCancellation,
  onSubmit,
}: InteractionFormProps & { supportsCancellation: boolean }) {
  return (
    <div class="space-y-3">
      <RequestDetails input={input} />
      <div class="flex flex-wrap gap-2">
        <DecisionButton disabled={disabled} onClick={() => onSubmit({ kind: "approve", scope: "once" })}>
          Allow once
        </DecisionButton>
        <DecisionButton disabled={disabled} onClick={() => onSubmit({ kind: "approve", scope: "session" })}>
          Allow for session
        </DecisionButton>
        <DecisionButton tone="danger" disabled={disabled} onClick={() => onSubmit({ kind: "deny_approval" })}>
          Deny
        </DecisionButton>
        {supportsCancellation && (
          <DecisionButton disabled={disabled} onClick={() => onSubmit({ kind: "cancel_approval" })}>Cancel request</DecisionButton>
        )}
      </div>
      <p class="text-[10px] text-ink-400">“Allow for session” applies to matching requests in this Codex session.</p>
    </div>
  );
}

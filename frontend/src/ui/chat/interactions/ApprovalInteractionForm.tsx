import { DecisionButton, RequestDetails } from "./InteractionControls";
import { summarizeApprovalRequest } from "./approvalSummary";
import type { InteractionFormProps } from "./types";

export function ApprovalInteractionForm({
  input,
  disabled,
  supportsCancellation,
  onSubmit,
}: InteractionFormProps & { supportsCancellation: boolean }) {
  const summary = summarizeApprovalRequest(input);
  return (
    <div class="space-y-3">
      {summary.reason && <p class="text-[13px] leading-relaxed text-ink-100">{summary.reason}</p>}
      {summary.command && (
        <div>
          <div class="mb-1 text-[10px] font-medium text-ink-400">Command</div>
          <pre class="overflow-x-auto whitespace-pre-wrap break-all rounded-control border border-line bg-canvas p-2 font-mono text-[11px] leading-relaxed text-ink-100">
            {summary.command}
          </pre>
          {summary.cwd && (
            <div class="mt-1 font-mono text-[10px] text-ink-400">in {summary.cwd}</div>
          )}
        </div>
      )}
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
      <p class="text-[10px] text-ink-400">“Allow for session” applies to matching requests in this agent session.</p>
    </div>
  );
}

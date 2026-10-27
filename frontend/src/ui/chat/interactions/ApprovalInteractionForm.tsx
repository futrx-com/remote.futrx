import type { ApprovalReviewAction } from "../../../models/chatInteraction";
import { useState } from "preact/hooks";
import { DecisionButton, RequestDetails } from "./InteractionControls";
import type { InteractionFormProps } from "./types";

export function ApprovalInteractionForm({
  input,
  disabled,
  supportsCancellation,
  onSubmit,
}: InteractionFormProps & { supportsCancellation: boolean }) {
  const [feedback, setFeedback] = useState("");
  const display = input.display as {
    kind?: string;
    plan?: string;
    options?: Array<{ label: string; description?: string }>;
  } | undefined;
  const plan = display?.kind === "plan_review" ? display : undefined;
  function decide(action: ApprovalReviewAction, optionLabel?: string) {
    onSubmit({ kind: "review_approval", action, feedback, optionLabel });
  }
  return (
    <div class="space-y-3">
      {plan ? (
        <pre class="max-h-96 overflow-auto whitespace-pre-wrap text-[12px] text-ink-200">{plan.plan}</pre>
      ) : <RequestDetails input={input} />}
      {input.allowFeedback === true && (
        <textarea
          aria-label="Feedback for the agent"
          placeholder="Optional feedback"
          value={feedback}
          disabled={disabled}
          onInput={(event) => setFeedback(event.currentTarget.value)}
          class="w-full rounded-control border border-line bg-canvas p-2 text-[12px] text-ink-100"
        />
      )}
      {plan?.options?.map((option) => (
        <DecisionButton key={option.label} disabled={disabled} onClick={() => decide("allow_once", option.label)}>
          {`${option.label}${option.description ? ` — ${option.description}` : ""}`}
        </DecisionButton>
      ))}
      <div class="flex flex-wrap gap-2">
        <DecisionButton disabled={disabled} onClick={() => input.allowFeedback ? decide("allow_once") : onSubmit({ kind: "approve", scope: "once" })}>
          Allow once
        </DecisionButton>
        <DecisionButton disabled={disabled} onClick={() => input.allowFeedback ? decide("allow_session") : onSubmit({ kind: "approve", scope: "session" })}>
          Allow for session
        </DecisionButton>
        <DecisionButton tone="danger" disabled={disabled} onClick={() => input.allowFeedback ? decide(plan ? "revise_plan" : "deny") : onSubmit({ kind: "deny_approval" })}>
          {plan ? "Request revisions" : "Deny"}
        </DecisionButton>
        {plan && (
          <DecisionButton tone="danger" disabled={disabled} onClick={() => decide("reject_plan")}>
            Reject and exit Plan mode
          </DecisionButton>
        )}
        {supportsCancellation && (
          <DecisionButton disabled={disabled} onClick={() => onSubmit({ kind: "cancel_approval" })}>Cancel request</DecisionButton>
        )}
      </div>
      <p class="text-[10px] text-ink-400">“Allow for session” applies to matching requests in this agent session.</p>
    </div>
  );
}

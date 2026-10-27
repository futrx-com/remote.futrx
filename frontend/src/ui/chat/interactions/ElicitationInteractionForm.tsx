import { useState } from "preact/hooks";
import { DecisionButton, RequestDetails } from "./InteractionControls";
import type { InteractionFormProps } from "./types";

export function ElicitationInteractionForm({
  input,
  disabled,
  onSubmit,
}: InteractionFormProps) {
  const [content, setContent] = useState("{}");
  const [error, setError] = useState("");

  function accept() {
    try {
      onSubmit({ kind: "accept_elicitation", content: JSON.parse(content) });
    } catch {
      setError("Enter valid JSON content.");
    }
  }

  return (
    <div class="space-y-3">
      <RequestDetails input={input} />
      <textarea
        value={content}
        disabled={disabled}
        onInput={(event) => setContent((event.currentTarget as HTMLTextAreaElement).value)}
        class="min-h-24 w-full rounded-control border border-line bg-canvas p-2 font-mono text-[11px] text-ink-100 outline-none focus:border-accent-blue"
        aria-label="Elicitation response JSON"
      />
      {error && <p class="text-[11px] text-accent-red">{error}</p>}
      <div class="flex flex-wrap gap-2">
        <DecisionButton disabled={disabled} onClick={accept}>Accept</DecisionButton>
        <DecisionButton tone="danger" disabled={disabled} onClick={() => onSubmit({ kind: "decline_elicitation" })}>Decline</DecisionButton>
        <DecisionButton disabled={disabled} onClick={() => onSubmit({ kind: "cancel_elicitation" })}>Cancel</DecisionButton>
      </div>
    </div>
  );
}

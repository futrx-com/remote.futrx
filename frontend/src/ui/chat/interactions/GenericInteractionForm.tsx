import { useState } from "preact/hooks";
import { DecisionButton, RequestDetails } from "./InteractionControls";
import type { InteractionFormProps } from "./types";

export function GenericInteractionForm({
  input,
  disabled,
  onSubmit,
}: InteractionFormProps) {
  const [result, setResult] = useState("null");
  const [error, setError] = useState("");

  function submit() {
    try {
      onSubmit({ kind: "submit_provider_result", result: JSON.parse(result) });
    } catch {
      setError("Enter a valid JSON result.");
    }
  }

  return (
    <div class="space-y-3">
      <p class="text-[12px] text-ink-300">This provider request has no dedicated Remote form. Review it and return an explicit JSON result.</p>
      <RequestDetails input={input} />
      <textarea
        value={result}
        disabled={disabled}
        onInput={(event) => setResult((event.currentTarget as HTMLTextAreaElement).value)}
        class="min-h-20 w-full rounded-control border border-line bg-canvas p-2 font-mono text-[11px] text-ink-100 outline-none focus:border-accent-blue"
        aria-label="Provider request result JSON"
      />
      {error && <p class="text-[11px] text-accent-red">{error}</p>}
      <div class="flex flex-wrap gap-2">
        <DecisionButton disabled={disabled} onClick={submit}>Send result</DecisionButton>
        <DecisionButton tone="danger" disabled={disabled} onClick={() => onSubmit({ kind: "decline_unsupported" })}>
          Decline as unsupported
        </DecisionButton>
      </div>
    </div>
  );
}

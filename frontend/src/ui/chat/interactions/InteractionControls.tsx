import { approvalSummaryFields } from "./approvalSummary";

export function RequestDetails({ input }: { input: Record<string, unknown> }) {
  const fields = approvalSummaryFields(input);

  return (
    <div class="space-y-2">
      {fields.length > 0 ? (
        <dl class="grid gap-2 rounded-control border border-line bg-canvas px-2.5 py-2 sm:grid-cols-3">
          {fields.map((field) => (
            <div class="min-w-0" key={field.label}>
              <dt class="text-[10px] font-semibold uppercase tracking-wide text-ink-400">{field.label}</dt>
              <dd class="mt-0.5 whitespace-pre-wrap break-words font-mono text-[11px] leading-relaxed text-ink-200">
                {field.value}
              </dd>
            </div>
          ))}
        </dl>
      ) : (
        <p class="rounded-control border border-line bg-canvas px-2.5 py-2 text-[11px] text-ink-400">
          No summary is available for this request.
        </p>
      )}
      <details class="rounded-control border border-line bg-canvas px-2.5 py-2">
        <summary class="cursor-pointer text-[11px] font-medium text-ink-300">Show full request details</summary>
        <pre class="mt-2 max-h-56 overflow-auto whitespace-pre-wrap break-all font-mono text-[10px] leading-relaxed text-ink-400">
          {JSON.stringify(input, null, 2)}
        </pre>
      </details>
    </div>
  );
}

export function DecisionButton({
  children,
  disabled,
  tone = "normal",
  onClick,
}: {
  children: string;
  disabled: boolean;
  tone?: "normal" | "danger";
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      class={`h-8 rounded-control border px-3 text-[11px] font-medium transition disabled:cursor-not-allowed disabled:opacity-50 ${tone === "danger" ? "border-accent-red/40 text-accent-red hover:bg-accent-red/10" : "border-line-strong text-ink-200 hover:bg-tint-strong"}`}
    >
      {children}
    </button>
  );
}

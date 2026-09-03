export function RequestDetails({ input }: { input: Record<string, unknown> }) {
  return (
    <details class="rounded-control border border-line bg-canvas px-2.5 py-2">
      <summary class="cursor-pointer text-[11px] font-medium text-ink-300">Request details and scope</summary>
      <pre class="mt-2 max-h-56 overflow-auto whitespace-pre-wrap break-all font-mono text-[10px] leading-relaxed text-ink-400">
        {JSON.stringify(input, null, 2)}
      </pre>
    </details>
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

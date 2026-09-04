export function TwoFactorRecoveryCodes({
  codes,
  onDismiss,
}: {
  codes: string[];
  onDismiss: () => void;
}) {
  return (
    <div class="rounded-md border border-accent-yellow/30 bg-accent-yellow/[0.08] p-3 space-y-2">
      <div class="text-[13px] font-medium text-ink-50">
        Save these recovery codes now — they won't be shown again.
      </div>
      <div class="grid grid-cols-2 gap-1.5 font-mono text-[12.5px] text-ink-100">
        {codes.map((code) => (
          <div key={code} class="bg-black/30 border border-white/10 rounded px-2 py-1">
            {code}
          </div>
        ))}
      </div>
      <button
        type="button"
        onClick={onDismiss}
        class="h-8 px-2.5 rounded bg-white/[0.08] hover:bg-white/[0.12] text-ink-100 text-[12px] font-medium"
      >
        I've saved these codes
      </button>
    </div>
  );
}

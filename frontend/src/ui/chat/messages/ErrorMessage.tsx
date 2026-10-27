import { AlertCircle } from "../../primitives/icons";
import { limitResetText } from "./limitResetText";

export function ErrorMessage({ message, t }: { message: string; t?: number }) {
  const nowMs = Date.now();
  // A usage-limit error names its reset in the container's UTC; show the viewer's time.
  const text = limitResetText.localize(message, { sentMs: t || nowMs, nowMs });
  return (
    <div class="flex items-start gap-2 text-accent-red text-sm rounded-card border border-accent-red/25 bg-accent-red/[0.08] px-3 py-2">
      <AlertCircle class="w-4 h-4 flex-none mt-0.5" />
      <div class="min-w-0 flex-1 [overflow-wrap:anywhere]" title={text === message ? undefined : message}>{text}</div>
    </div>
  );
}

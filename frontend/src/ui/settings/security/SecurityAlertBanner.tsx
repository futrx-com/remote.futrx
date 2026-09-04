import { useState } from "preact/hooks";
import type { SecurityAlertSummary } from "../../../models/security";
import { AlertCircle } from "../../primitives/icons";

export function SecurityAlertBanner({
  alert,
  onAck,
}: {
  alert: SecurityAlertSummary;
  onAck: () => Promise<void>;
}) {
  const [acking, setAcking] = useState(false);
  return (
    <div class="rounded-lg border border-accent-yellow/30 bg-accent-yellow/[0.08] p-3 flex items-start gap-3">
      <AlertCircle class="w-4 h-4 text-accent-yellow flex-none mt-0.5" />
      <div class="flex-1 min-w-0">
        <div class="text-[13px] font-medium text-ink-50">
          A recent sign-in used a recovery code instead of your authenticator app.
        </div>
        <div class="text-[12px] text-ink-300 mt-1">
          {new Date(alert.occurredAt * 1000).toLocaleString()} · {alert.ip || "unknown IP"}
        </div>
      </div>
      <button
        type="button"
        disabled={acking}
        onClick={async () => {
          setAcking(true);
          try {
            await onAck();
          } finally {
            setAcking(false);
          }
        }}
        class="h-8 px-2.5 rounded bg-white/[0.08] hover:bg-white/[0.12] text-ink-100 text-[12px] font-medium disabled:opacity-50"
      >
        Acknowledge
      </button>
    </div>
  );
}

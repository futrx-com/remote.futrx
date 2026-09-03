import type { SessionHistoryEntry } from "../../../models/security";
import { Clock } from "../../primitives/icons";

export function SessionHistoryList({ sessions }: { sessions: SessionHistoryEntry[] }) {
  if (!sessions?.length) {
    return (
      <div class="text-[12.5px] text-ink-400">No sign-ins recorded yet.</div>
    );
  }
  return (
    <div class="rounded-lg border border-white/10 bg-[#101318] overflow-hidden">
      <div class="px-3.5 py-2.5 border-b border-white/[0.06] flex items-center gap-2">
        <Clock class="w-4 h-4 text-ink-300" />
        <div class="text-[13px] font-medium text-ink-100">Recent sign-ins</div>
      </div>
      <ul class="divide-y divide-white/[0.06]">
        {sessions.map((entry) => (
          <li key={entry.sid} class="px-3.5 py-2.5 text-[12.5px] text-ink-200 flex items-center justify-between gap-3">
            <div class="min-w-0">
              <div class="truncate">{entry.method}{entry.ip ? ` · ${entry.ip}` : ""}</div>
              <div class="text-[11.5px] text-ink-400 truncate">{entry.userAgent || "unknown device"}</div>
            </div>
            <div class="text-[11.5px] text-ink-400 flex-none">
              {new Date(entry.issuedAt * 1000).toLocaleString()}
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}

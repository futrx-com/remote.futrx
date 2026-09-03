import { Check } from "../../../primitives/icons";

export function AnsweredSummary({ answered }: { answered: string }) {
  return (
    <div class="my-2 rounded-lg border border-line bg-tint p-3 text-sm">
      <div class="flex items-center gap-2 text-ink-300 text-[11px] mb-1.5">
        <Check class="w-3 h-3 text-accent-green" /> Answered
      </div>
      <div class="text-ink-200 whitespace-pre-wrap text-[13px] [overflow-wrap:anywhere]">{answered}</div>
    </div>
  );
}

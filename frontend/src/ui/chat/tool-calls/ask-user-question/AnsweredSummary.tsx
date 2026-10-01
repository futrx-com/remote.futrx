import { Check } from "../../../primitives/icons";
import { answeredText } from "./answeredText";

export function AnsweredSummary({ answered }: { answered: string }) {
  const pairs = answeredText.parse(answered);
  return (
    <div class="my-2 rounded-lg border border-line bg-tint p-3 text-sm">
      <div class="flex items-center gap-2 text-ink-300 text-[11px] mb-1.5">
        <Check class="w-3 h-3 text-accent-green" /> Answered
      </div>
      {pairs ? (
        <div class="space-y-2.5">
          {pairs.map((pair, index) => (
            <div key={index} class="space-y-0.5 [overflow-wrap:anywhere]">
              <div class="text-ink-100 text-[13px] font-medium whitespace-pre-wrap">{pair.question}</div>
              <div class="text-ink-200 text-[13px] whitespace-pre-wrap">{pair.answer || "No answer"}</div>
            </div>
          ))}
        </div>
      ) : (
        <div class="text-ink-200 whitespace-pre-wrap text-[13px] [overflow-wrap:anywhere]">{answered}</div>
      )}
    </div>
  );
}

import { Skeleton } from "../../primitives/Skeleton";

// A transcript in outline: a short prompt bubble on the right, a longer answer
// running full width on the left. Prompts carry UserMessage's own rounding
// (panel, squared off at the corner nearest the sender) so the bubble does not
// change shape when the real message arrives. Every last paragraph line is
// short, which is what makes a block of bars read as prose.
const PLACEHOLDER_TURNS = [
  { prompt: "w-[34%]", reply: ["w-[94%]", "w-[88%]", "w-[52%]"] },
  { prompt: "w-[52%]", reply: ["w-[91%]", "w-[96%]", "w-[84%]", "w-[41%]"] },
  { prompt: "w-[43%]", reply: ["w-[89%]", "w-[73%]"] },
];

export function MessageSkeleton() {
  return (
    <div role="status" aria-label="Loading conversation" class="space-y-6 md:space-y-7">
      {PLACEHOLDER_TURNS.map((turn, turnIndex) => (
        <div key={turnIndex} class="space-y-5 md:space-y-6">
          <div class="flex justify-end">
            <Skeleton class={`h-10 rounded-panel rounded-br-control ${turn.prompt}`} />
          </div>
          <div class="space-y-3">
            {turn.reply.map((width, lineIndex) => (
              <Skeleton key={lineIndex} class={`h-2.5 ${width}`} />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

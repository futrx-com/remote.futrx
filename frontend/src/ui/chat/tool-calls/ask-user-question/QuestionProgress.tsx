import type { Question } from "../../../../models/chat.ts";

export function QuestionProgress({
  questions,
  page,
  total,
  questionAnswered,
}: {
  questions: Question[];
  page: number;
  total: number;
  questionAnswered: (index: number) => boolean;
}) {
  if (total <= 1) return null;

  return (
    <span class="flex items-center gap-1.5 text-accent-blue/80">
      <span class="font-mono text-[10.5px]">{page + 1} / {total}</span>
      <span class="flex gap-1">
        {questions.map((_, index) => (
          <span
            key={index}
            class={`w-1.5 h-1.5 rounded-full
              ${index === page ? "bg-accent-blue"
                : questionAnswered(index) ? "bg-accent-green" : "bg-accent-blue/30"}`}
          />
        ))}
      </span>
    </span>
  );
}

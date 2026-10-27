import type { ChatInteractionQuestion } from "../../../models/chatInteraction";

/**
 * A resolved requestUserInput card: each question the agent asked, with the
 * answer the user gave. Answers are only as complete as the transcript: secret
 * answers are never saved, and chats from before answers were recorded have
 * none.
 */
export function AnsweredUserInput({
  questions,
  answers,
  note,
}: {
  questions: ChatInteractionQuestion[];
  answers?: Record<string, string[]>;
  /** Shown under the questions when the request ended without an answer. */
  note?: string;
}) {
  return (
    <div class="space-y-3">
      {questions.map((question, index) => {
        const id = question.id || String(index);
        const answer = (answers?.[id] ?? []).filter((value) => value.trim()).join(", ");
        return (
          <div key={id} class="space-y-1.5">
            <p class="text-[13px] font-medium leading-snug text-ink-100">
              {question.header && <span class="mr-2 font-mono text-[10px] text-ink-400">{question.header}</span>}
              {question.question || "The agent requested input"}
            </p>
            {question.isSecret ? (
              <p class="text-[12px] italic text-ink-400">Secret answer, not saved to chat history</p>
            ) : answer ? (
              <p class="inline-block rounded-control border border-accent-blue/40 bg-accent-blue/10 px-2.5 py-1.5 text-[12px] text-ink-100 [overflow-wrap:anywhere]">
                {answer}
              </p>
            ) : (
              <p class="text-[12px] italic text-ink-400">No answer recorded</p>
            )}
          </div>
        );
      })}
      {note && <p class="text-[12px] text-ink-300">{note}</p>}
    </div>
  );
}

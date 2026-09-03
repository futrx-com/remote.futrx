import { useState } from "preact/hooks";
import type { ChatInteractionQuestion } from "../../../models/chatInteraction";
import { DecisionButton } from "./InteractionControls";
import type { InteractionFormProps } from "./types";

export function UserInputInteractionForm({
  input,
  disabled,
  onSubmit,
}: InteractionFormProps) {
  const questions = Array.isArray(input.questions) ? input.questions as ChatInteractionQuestion[] : [];
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const complete = questions.length > 0 && questions.every((question, index) => {
    const id = question.id || String(index);
    return (answers[id] || "").trim().length > 0;
  });

  function submit() {
    const encoded: Record<string, string[]> = {};
    questions.forEach((question, index) => {
      const id = question.id || String(index);
      encoded[id] = [(answers[id] || "").trim()];
    });
    onSubmit({ kind: "answer_questions", answers: encoded });
  }

  return (
    <div class="space-y-4">
      {typeof input.autoResolutionMs === "number" && (
        <p class="text-[10px] text-ink-400">
          Codex may auto-resolve this request after {Math.ceil(input.autoResolutionMs / 1000)} seconds.
        </p>
      )}
      {questions.map((question, index) => {
        const id = question.id || String(index);
        const options = Array.isArray(question.options) ? question.options : [];
        return (
          <fieldset key={id} class="space-y-2" disabled={disabled}>
            <legend class="text-[13px] font-medium leading-snug text-ink-100">
              {question.header && <span class="mr-2 font-mono text-[10px] text-ink-400">{question.header}</span>}
              {question.question || "Codex is requesting input"}
            </legend>
            {options.length > 0 && (
              <div class="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
                {options.map((option, optionIndex) => (
                  <button
                    key={`${id}-${optionIndex}`}
                    type="button"
                    onClick={() => setAnswers((current) => ({ ...current, [id]: option.label || "" }))}
                    class={`rounded-control border px-2.5 py-2 text-left text-[12px] transition ${answers[id] === option.label ? "border-accent-blue bg-accent-blue/10 text-ink-100" : "border-line bg-surface text-ink-200 hover:border-line-strong"}`}
                  >
                    <span class="block font-medium">{option.label}</span>
                    {option.description && <span class="mt-0.5 block text-[10px] text-ink-400">{option.description}</span>}
                  </button>
                ))}
              </div>
            )}
            {(options.length === 0 || question.isOther) && (
              <input
                type={question.isSecret ? "password" : "text"}
                value={answers[id] || ""}
                autocomplete="off"
                placeholder={question.isSecret ? "Secret answer (not saved to chat history)" : "Type an answer"}
                onInput={(event) => setAnswers((current) => ({
                  ...current,
                  [id]: (event.currentTarget as HTMLInputElement).value,
                }))}
                class="h-9 w-full rounded-control border border-line bg-canvas px-2.5 text-[12px] text-ink-100 outline-none focus:border-accent-blue"
              />
            )}
          </fieldset>
        );
      })}
      <DecisionButton disabled={disabled || !complete} onClick={submit}>Send answers</DecisionButton>
    </div>
  );
}

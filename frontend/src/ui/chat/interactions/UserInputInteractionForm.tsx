import { useQuestionForm } from "../../../state/hooks/chat/useQuestionForm";
import { DecisionButton } from "./InteractionControls";
import type { InteractionFormProps } from "./types";

export function UserInputInteractionForm({ input, disabled, onSubmit }: InteractionFormProps) {
  const form = useQuestionForm(input);
  function submit() {
    onSubmit({ kind: "answer_questions", answers: form.answers() });
  }
  return (
    <div class="space-y-4">
      {typeof input.autoResolutionMs === "number" && (
        <p class="text-[10px] text-ink-400">The agent may auto-resolve this request after {Math.ceil(input.autoResolutionMs / 1000)} seconds.</p>
      )}
      {form.questions.map((row) => {
        const { id, question, options } = row;
        return (
          <fieldset key={id} class="space-y-2" disabled={disabled}>
            <legend class="text-[13px] font-medium leading-snug text-ink-100">
              {question.header && <span class="mr-2 font-mono text-[10px] text-ink-400">{question.header}</span>}
              {question.question || "The agent is requesting input"}
            </legend>
            {question.body && <p class="whitespace-pre-wrap text-[12px] text-ink-200">{question.body}</p>}
            {question.multiSelect && <p class="text-[10px] text-ink-400">Select all that apply.</p>}
            {options.length > 0 && (
              <div class="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
                {options.map((option, optionIndex) => {
                  const selected = option.selected;
                  return (
                    <button key={`${id}-${optionIndex}`} type="button" aria-pressed={selected}
                      onClick={() => form.select(row, option)}
                      class={`rounded-control border px-2.5 py-2 text-left text-[12px] transition ${selected ? "border-accent-blue bg-accent-blue/10 text-ink-100" : "border-line bg-surface text-ink-200 hover:border-line-strong"}`}>
                      <span class="block font-medium">{option.label}</span>
                      {option.description && <span class="mt-0.5 block text-[10px] text-ink-400">{option.description}</span>}
                    </button>
                  );
                })}
              </div>
            )}
            {(options.length === 0 || question.isOther) && (
              <input type={question.isSecret ? "password" : "text"} value={row.other} autocomplete="off"
                placeholder={question.isSecret ? "Secret answer (not saved to chat history)" : "Type an answer"}
                onInput={(event) => form.enterText(row, event.currentTarget.value)}
                class="h-9 w-full rounded-control border border-line bg-canvas px-2.5 text-[12px] text-ink-100 outline-none focus:border-accent-blue" />
            )}
          </fieldset>
        );
      })}
      <div class="flex gap-2">
        <DecisionButton disabled={disabled || !form.complete} onClick={submit}>Send answers</DecisionButton>
        {input.allowDismiss === true && (
          <DecisionButton disabled={disabled} onClick={() => onSubmit({ kind: "dismiss_questions" })}>
            Dismiss
          </DecisionButton>
        )}
      </div>
    </div>
  );
}

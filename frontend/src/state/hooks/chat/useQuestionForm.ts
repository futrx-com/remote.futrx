import { useState } from "preact/hooks";
import type { ChatInteractionQuestion } from "../../../models/chatInteraction";
import { QuestionAnswerState, type QuestionAnswerView } from "./questionAnswerState";

export function useQuestionForm(input: Record<string, unknown>) {
  const questions = Array.isArray(input.questions) ? input.questions as ChatInteractionQuestion[] : [];
  const [state, setState] = useState(() => new QuestionAnswerState());
  const rows = state.view(questions);
  return {
    questions: rows,
    complete: state.complete(rows),
    answers: () => state.responses(rows),
    select: (row: QuestionAnswerView, option: QuestionAnswerView["options"][number]) =>
      setState((current) => current.select(row, option)),
    enterText: (row: QuestionAnswerView, text: string) =>
      setState((current) => current.enterText(row, text)),
  };
}

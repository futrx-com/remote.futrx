import type { ChatInteractionQuestion } from "../../../models/chatInteraction";

interface QuestionOptionView {
  value: string;
  label?: string;
  description?: string;
  selected: boolean;
}

export interface QuestionAnswerView {
  id: string;
  question: ChatInteractionQuestion;
  options: QuestionOptionView[];
  other: string;
}

// Component-scoped, immutable answer state. Selection and free text transition
// together so a single-choice question cannot retain both after an edit.
export class QuestionAnswerState {
  private readonly answers: Record<string, string[]>;
  private readonly other: Record<string, string>;

  constructor(answers: Record<string, string[]> = {}, other: Record<string, string> = {}) {
    this.answers = answers;
    this.other = other;
  }

  view(questions: ChatInteractionQuestion[]): QuestionAnswerView[] {
    return questions.map((question, index) => {
      const id = question.id || String(index);
      const options = Array.isArray(question.options) ? question.options : [];
      return {
        id,
        question,
        other: this.other[id] || "",
        options: options.map((option) => {
          const value = option.id ?? option.label ?? "";
          return { ...option, value, selected: this.answers[id]?.includes(value) ?? false };
        }),
      };
    });
  }

  select(row: QuestionAnswerView, option: QuestionOptionView): QuestionAnswerState {
    const { id, question } = row;
    const { value, selected } = option;
    const answers = {
      ...this.answers,
      [id]: question.multiSelect
        ? (selected ? (this.answers[id] || []).filter((item) => item !== value) : [...(this.answers[id] || []), value])
        : [value],
    };
    const other = question.multiSelect ? this.other : { ...this.other, [id]: "" };
    return new QuestionAnswerState(answers, other);
  }

  enterText(row: QuestionAnswerView, text: string): QuestionAnswerState {
    const other = { ...this.other, [row.id]: text };
    const answers = row.question.multiSelect ? this.answers : { ...this.answers, [row.id]: [] };
    return new QuestionAnswerState(answers, other);
  }

  complete(rows: QuestionAnswerView[]): boolean {
    return rows.length > 0 && rows.every(({ id }) =>
      (this.answers[id]?.length ?? 0) > 0 || (this.other[id] || "").trim().length > 0
    );
  }

  responses(rows: QuestionAnswerView[]): Record<string, string[]> {
    const encoded: Record<string, string[]> = {};
    for (const { id } of rows) {
      const text = (this.other[id] || "").trim();
      encoded[id] = [...(this.answers[id] || []), ...(text ? [text] : [])];
    }
    return encoded;
  }
}

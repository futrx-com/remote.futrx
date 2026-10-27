import { useEffect, useMemo, useState } from "preact/hooks";
import { readAnswered, writeAnswered } from "./storage";
import type { AskUserQuestionInput } from "./types";

function createSelectionState(count: number): Record<number, Set<number>> {
  const init: Record<number, Set<number>> = {};
  for (let i = 0; i < count; i++) init[i] = new Set();
  return init;
}

export function useAskUserQuestion({
  toolUseId,
  input,
  onSubmit,
}: {
  toolUseId: string;
  input: AskUserQuestionInput;
  onSubmit: (text: string) => boolean;
}) {
  const questions = input.questions ?? [];
  const total = questions.length;
  const initialAnswered = useMemo(() => readAnswered(toolUseId), [toolUseId]);
  const [answered, setAnswered] = useState<string | null>(initialAnswered);
  const [sendFailed, setSendFailed] = useState(false);
  const [page, setPage] = useState(0);
  const [selections, setSelections] = useState<Record<number, Set<number>>>(() =>
    createSelectionState(total)
  );
  const [other, setOther] = useState<Record<number, string>>({});
  const [otherActive, setOtherActive] = useState<Record<number, boolean>>({});

  useEffect(() => {
    setAnswered(initialAnswered);
  }, [initialAnswered]);

  useEffect(() => {
    setPage(0);
    setSelections(createSelectionState(total));
    setOther({});
    setOtherActive({});
  }, [toolUseId, total]);

  function toggle(qi: number, oi: number, multi: boolean) {
    setSelections((prev) => {
      const next = { ...prev };
      const set = new Set(next[qi] ?? []);
      if (multi) {
        if (set.has(oi)) set.delete(oi);
        else set.add(oi);
      } else {
        set.clear();
        set.add(oi);
      }
      next[qi] = set;
      return next;
    });
    setOtherActive((prev) => ({ ...prev, [qi]: false }));
  }

  function activateOther(qi: number, multi: boolean) {
    setOtherActive((prev) => ({ ...prev, [qi]: true }));
    if (!multi) {
      setSelections((prev) => ({ ...prev, [qi]: new Set() }));
    }
  }

  function setOtherText(qi: number, value: string) {
    setOther((prev) => ({ ...prev, [qi]: value }));
  }

  function questionAnswered(qi: number): boolean {
    const sel = selections[qi] ?? new Set<number>();
    const otherText = otherActive[qi] ? (other[qi] || "").trim() : "";
    if (otherText.length > 0) return true;
    return sel.size > 0;
  }

  function chosenLabels(qi: number): string[] {
    const question = questions[qi];
    if (!question) return [];
    const chosen: string[] = [];
    (selections[qi] ?? new Set<number>()).forEach((optionIndex) => {
      const option = question.options[optionIndex];
      if (option) chosen.push(option.label);
    });
    if (otherActive[qi]) {
      const text = (other[qi] || "").trim();
      if (text) chosen.push(text);
    }
    return chosen;
  }

  /** The answer as the agent receives it; the answered card parses it back. */
  function summarize(): string {
    const parts: string[] = [];
    for (let qi = 0; qi < questions.length; qi++) {
      parts.push(`Q: ${questions[qi].question}\nA: ${chosenLabels(qi).join("; ")}`);
    }
    return parts.join("\n\n");
  }

  function submit() {
    const text = summarize();
    // Only a delivered answer counts. Marking the card first left it saying
    // "Answered" after a dropped connection, with nothing sent and no retry.
    if (!onSubmit(text)) {
      setSendFailed(true);
      return;
    }
    setSendFailed(false);
    // The full text, so the answered card still shows what was asked.
    writeAnswered(toolUseId, text);
    setAnswered(text);
  }

  return {
    questions,
    total,
    answered,
    sendFailed,
    page,
    setPage,
    currentQuestion: questions[page],
    selectedOptions: selections[page] ?? new Set<number>(),
    isOtherActive: !!otherActive[page],
    otherText: other[page] || "",
    canAdvance: questionAnswered(page),
    questionAnswered,
    toggle,
    activateOther,
    setOtherText,
    submit,
  };
}

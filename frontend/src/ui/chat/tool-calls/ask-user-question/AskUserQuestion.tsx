// Render agent AskUserQuestion tool calls as a paginated wizard.

import { AnsweredSummary } from "./AnsweredSummary";
import { OtherAnswerOption } from "./OtherAnswerOption";
import { QuestionOption } from "./QuestionOption";
import { QuestionPager } from "./QuestionPager";
import { QuestionProgress } from "./QuestionProgress";
import type { AskUserQuestionInput } from "./types";
import { useAskUserQuestion } from "./useAskUserQuestion";

interface Props {
  toolUseId: string;
  chatId: string;
  input: AskUserQuestionInput;
  onSubmit: (text: string) => void;
}

export function AskUserQuestion({ toolUseId, input, onSubmit }: Props) {
  const wizard = useAskUserQuestion({ toolUseId, input, onSubmit });
  const question = wizard.currentQuestion;

  if (!wizard.questions.length || !question) return null;
  if (wizard.answered) return <AnsweredSummary answered={wizard.answered} />;

  const multi = !!question.multiSelect;

  return (
    <div class="my-2 rounded-lg border border-accent-blue/40 bg-accent-blue/5 overflow-hidden">
      <div class="px-3 py-2 bg-accent-blue/10 text-[11px] text-accent-blue
                  flex items-center justify-between gap-2 border-b border-accent-blue/20">
        <span>Agent is asking</span>
        <QuestionProgress
          questions={wizard.questions}
          page={wizard.page}
          total={wizard.total}
          questionAnswered={wizard.questionAnswered}
        />
      </div>

      <div class="p-3 space-y-3">
        {question.header && (
          <div class="inline-block text-[10px] font-mono
                      px-1.5 py-0.5 rounded bg-tint text-ink-200 border border-line">
            {question.header}
          </div>
        )}
        <div class="text-[14px] text-ink-100 font-medium leading-snug">{question.question}</div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
          {question.options.map((option, index) => (
            <QuestionOption
              key={index}
              label={option.label}
              description={option.description}
              active={wizard.selectedOptions.has(index) && !wizard.isOtherActive}
              multi={multi}
              onClick={() => wizard.toggle(wizard.page, index, multi)}
            />
          ))}
          <OtherAnswerOption
            active={wizard.isOtherActive}
            multi={multi}
            value={wizard.otherText}
            onActivate={() => wizard.activateOther(wizard.page, multi)}
            onChange={(value) => wizard.setOtherText(wizard.page, value)}
          />
        </div>

        <QuestionPager
          page={wizard.page}
          total={wizard.total}
          canAdvance={wizard.canAdvance}
          onPageChange={wizard.setPage}
          onSubmit={wizard.submit}
        />
      </div>
    </div>
  );
}

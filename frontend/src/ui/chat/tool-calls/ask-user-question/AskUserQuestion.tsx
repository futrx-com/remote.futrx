// Render agent AskUserQuestion tool calls as a paginated wizard.

import type {
  AskUserQuestionInput,
  QuestionAnswerSubmission,
} from "../../../../models/chat.ts";
import { chatQuestionCountdownService } from "../../../../services/chat/chatQuestionCountdownService.ts";
import { chatQuestionService } from "../../../../services/chat/chatQuestionService.ts";
import { useAskUserQuestion } from "../../../../state/hooks/chat/useAskUserQuestion.ts";
import { useAutoResolutionCountdown } from "../../../../state/hooks/chat/useAutoResolutionCountdown.ts";
import { AnsweredSummary } from "./AnsweredSummary";
import { OtherAnswerOption } from "./OtherAnswerOption";
import { QuestionOption } from "./QuestionOption";
import { QuestionPager } from "./QuestionPager";
import { QuestionProgress } from "./QuestionProgress";

interface Props {
  toolUseId: string;
  chatId: string;
  input: AskUserQuestionInput;
  onSubmit: (answer: QuestionAnswerSubmission) => boolean;
  onActivity?: () => boolean;
  interactive?: boolean;
  requestedAt?: number;
  resolved?: boolean;
  resolvedOutput?: string;
}

export function AskUserQuestion({
  toolUseId,
  chatId,
  input,
  onSubmit,
  interactive = false,
  requestedAt,
  resolved = false,
  resolvedOutput,
  onActivity,
}: Props) {
  const wizard = useAskUserQuestion({
    chatId,
    toolUseId,
    input,
    onSubmit,
    awaitResolution: interactive,
    allowOptionNotes: interactive,
    onActivity,
  });
  const autoResolutionRemaining = useAutoResolutionCountdown(
    interactive && input.isBlocking === false && !resolved && !wizard.autoResolutionSnoozed,
    requestedAt,
  );
  const question = wizard.currentQuestion;

  if (!wizard.questions.length || !question) return null;
  // A correlated backend resolution is authoritative over browser-local
  // optimistic state. The response can lose a race with native auto-resolution
  // or cancellation even when WebSocket.send() returned true.
  if (resolved) {
    return (
      <AnsweredSummary
        answered={chatQuestionService.resolvedPreview(wizard.questions, resolvedOutput)}
      />
    );
  }
  if (!interactive && wizard.questions.some((item) => item.isSecret)) {
    return (
      <div class="my-2 rounded-lg border border-accent-red/30 bg-accent-red/5 p-3 text-sm">
        <div class="text-accent-red text-[12px] font-medium">Secure response unavailable</div>
        <div class="mt-1 text-ink-300 text-[12px]">
          This completed agent transport cannot receive a secret without adding it to Remote chat
          history. Restart the task with a native interactive question instead.
        </div>
      </div>
    );
  }
  if (wizard.answered) return <AnsweredSummary answered={wizard.answered} />;
  if (interactive && !chatQuestionService.hasValidIds(wizard.questions)) {
    return (
      <div class="my-2 rounded-lg border border-accent-red/30 bg-accent-red/5 p-3 text-sm">
        <div class="text-accent-red text-[12px] font-medium">Question cannot be answered</div>
        <div class="mt-1 text-ink-300 text-[12px]">
          The agent did not provide unique response identifiers. Restart the turn and try again.
        </div>
      </div>
    );
  }

  const multi = !!question.multiSelect;
  const hasOptions = (question.options?.length ?? 0) > 0;
  const showFreeform = !hasOptions || interactive || question.isOther !== false;

  return (
    <div
      class="my-2 rounded-lg border border-accent-blue/40 bg-accent-blue/5 overflow-hidden"
      onKeyDown={wizard.reportActivity}
      onPaste={wizard.reportActivity}
    >
      <div class="px-3 py-2 bg-accent-blue/10 text-[11px] text-accent-blue
                  flex items-center justify-between gap-2 border-b border-accent-blue/20">
        <span>Agent is asking</span>
        <div class="flex items-center gap-2">
          {autoResolutionRemaining !== null && (
            <span class="text-accent-red font-medium">
              Auto-resolves in {chatQuestionCountdownService.formatRemaining(
                autoResolutionRemaining,
              )}
            </span>
          )}
          <QuestionProgress
            questions={wizard.questions}
            page={wizard.page}
            total={wizard.total}
            questionAnswered={wizard.questionAnswered}
          />
        </div>
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
          {(question.options ?? []).map((option, index) => (
            <QuestionOption
              key={index}
              label={option.label}
              description={option.description}
              active={wizard.selectedOptions.has(index) && (!wizard.isOtherActive || interactive)}
              multi={multi}
              onClick={() => wizard.toggle(wizard.page, index, multi)}
            />
          ))}
          {showFreeform && (
            <OtherAnswerOption
              active={wizard.isOtherActive}
              multi={multi || (interactive && hasOptions)}
              secret={!!question.isSecret}
              freeformOnly={!hasOptions}
              noteMode={interactive && hasOptions}
              value={wizard.otherText}
              onActivate={() => wizard.activateOther(wizard.page, multi)}
              onChange={(value) => wizard.setOtherText(wizard.page, value)}
            />
          )}
        </div>

        <QuestionPager
          page={wizard.page}
          total={wizard.total}
          canAdvance={wizard.canAdvance}
          onPageChange={wizard.setPage}
          onSubmit={wizard.submit}
        />
        {wizard.submissionError && (
          <div class="rounded border border-accent-red/30 bg-accent-red/5 px-2 py-1.5 text-[12px] text-accent-red">
            {wizard.submissionError}
          </div>
        )}
      </div>
    </div>
  );
}

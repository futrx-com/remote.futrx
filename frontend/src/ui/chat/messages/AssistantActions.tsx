import type { ComponentChildren } from "preact";
import { useCopyToClipboard } from "../../../state/hooks/shared/useCopyToClipboard";
import { Check, Copy, GitFork, RotateCcw } from "../../primitives/icons";
import type { AssistantTurnActions } from "./turnActions";

export function AssistantActions({
  actions,
  onRetry,
  onBranch,
}: {
  actions: AssistantTurnActions;
  onRetry?: (t: number, text: string) => void;
  onBranch?: (beforeT?: number) => void;
}) {
  const { copied, copy } = useCopyToClipboard();
  const { prompt } = actions;
  return (
    <div class="flex items-center gap-0.5 -ml-1.5 md:opacity-0 md:group-hover:opacity-100 md:group-focus-within:opacity-100 transition-opacity">
      <ActionButton label={copied ? "Copied" : "Copy"} onClick={() => void copy(actions.text)}>
        {copied ? <Check class="w-3.5 h-3.5 text-accent-green" /> : <Copy class="w-3.5 h-3.5" />}
      </ActionButton>
      {onRetry && prompt && (
        <ActionButton label="Retry" onClick={() => onRetry(prompt.t, prompt.text)}>
          <RotateCcw class="w-3.5 h-3.5" />
        </ActionButton>
      )}
      {onBranch && (
        <ActionButton label="Branch to a new chat" onClick={() => onBranch(actions.branchBeforeT)}>
          <GitFork class="w-3.5 h-3.5" />
        </ActionButton>
      )}
    </div>
  );
}

function ActionButton({
  label,
  onClick,
  children,
}: {
  label: string;
  onClick: () => void;
  children: ComponentChildren;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      class="h-7 w-7 rounded-control grid place-items-center text-ink-400 transition-colors
             hover:bg-tint-strong hover:text-ink-100"
      aria-label={label}
      title={label}
    >
      {children}
    </button>
  );
}

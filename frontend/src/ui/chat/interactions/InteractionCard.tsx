import { useState } from "preact/hooks";
import { ApprovalInteractionForm } from "./ApprovalInteractionForm";
import { ElicitationInteractionForm } from "./ElicitationInteractionForm";
import { GenericInteractionForm } from "./GenericInteractionForm";
import { PermissionInteractionForm } from "./PermissionInteractionForm";
import type { InteractionPart } from "./types";
import { UserInputInteractionForm } from "./UserInputInteractionForm";
import type { ChatInteractionIntent } from "../../../models/chatInteraction";
import type { ChatInteractionResponder } from "../../../types/chatApi";

export function InteractionCard({
  part,
  onRespond,
}: {
  part: InteractionPart;
  onRespond?: ChatInteractionResponder;
}) {
  const [submitting, setSubmitting] = useState(false);
  const [localError, setLocalError] = useState("");

  function respond(intent: ChatInteractionIntent) {
    if (!onRespond || submitting || part.status !== "pending") return;
    if (!onRespond(part.id, part.method, intent)) {
      setLocalError("The interaction response could not be sent. Check the connection and retry.");
      return;
    }
    setLocalError("");
    setSubmitting(true);
  }

  return (
    <section class="my-2 overflow-hidden rounded-lg border border-accent-blue/35 bg-accent-blue/[0.05]">
      <header class="flex items-center justify-between gap-3 border-b border-accent-blue/20 bg-accent-blue/[0.08] px-3 py-2">
        <div class="min-w-0">
          <div class="text-[11px] font-semibold text-accent-blue">Codex needs your decision</div>
          <div class="truncate font-mono text-[10px] text-ink-400">{part.method}</div>
        </div>
        <span class="rounded-full border border-line px-2 py-0.5 text-[10px] text-ink-300">
          {submitting && part.status === "pending" ? "sending" : part.status}
        </span>
      </header>
      <div class="p-3">
        {part.status !== "pending" ? (
          <p class="text-[12px] text-ink-300">Request {humanStatus(part.status)}.</p>
        ) : part.interactionKind === "user_input" ? (
          <UserInputInteractionForm input={part.input} disabled={submitting} onSubmit={respond} />
        ) : part.interactionKind === "approval" ? (
          <ApprovalInteractionForm
            input={part.input}
            disabled={submitting}
            supportsCancellation={part.supportsCancellation}
            onSubmit={respond}
          />
        ) : part.interactionKind === "permission" ? (
          <PermissionInteractionForm input={part.input} disabled={submitting} onSubmit={respond} />
        ) : part.interactionKind === "elicitation" ? (
          <ElicitationInteractionForm input={part.input} disabled={submitting} onSubmit={respond} />
        ) : (
          <GenericInteractionForm input={part.input} disabled={submitting} onSubmit={respond} />
        )}
        {localError && <p class="mt-2 text-[11px] text-accent-red">{localError}</p>}
      </div>
    </section>
  );
}

function humanStatus(status: string): string {
  return status.replaceAll("_", " ");
}

import { DecisionButton, RequestDetails } from "./InteractionControls";
import type { InteractionFormProps } from "./types";

export function PermissionInteractionForm({
  input,
  disabled,
  onSubmit,
}: InteractionFormProps) {
  const permissions = isRecord(input.permissions) ? input.permissions : {};
  return (
    <div class="space-y-3">
      {typeof input.reason === "string" && <p class="text-[13px] text-ink-200">{input.reason}</p>}
      <RequestDetails input={input} />
      <div class="flex flex-wrap gap-2">
        <DecisionButton disabled={disabled} onClick={() => onSubmit({ kind: "grant_permissions", permissions, scope: "turn" })}>Grant for turn</DecisionButton>
        <DecisionButton disabled={disabled} onClick={() => onSubmit({ kind: "grant_permissions", permissions, scope: "session" })}>Grant for session</DecisionButton>
        <DecisionButton tone="danger" disabled={disabled} onClick={() => onSubmit({ kind: "deny_permissions" })}>Deny</DecisionButton>
      </div>
    </div>
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

import {
  APPROVAL_POLICY_OPTIONS,
  SANDBOX_POLICY_OPTIONS,
} from "../../../config/chat";
import { Activity, Cpu, Lock, MessageSquare, ShieldCheck } from "../../primitives/icons";
import type { AgentCapabilityOption } from "../../../models/agentCapabilities";
import { ComposerOptionDropdown } from "./ComposerOptionDropdown";
import type { ComposerPreferenceActions, ComposerPreferences } from "./preferences";

export function ComposerExecutionControls({
  preferences,
  preferenceActions,
  streaming,
  reasoningEffortOptions,
  serviceTierOptions,
  modeOptions,
  supportsExecutionPolicies,
}: {
  preferences: ComposerPreferences;
  preferenceActions: ComposerPreferenceActions;
  streaming: boolean;
  reasoningEffortOptions: readonly { value: string; label: string }[];
  serviceTierOptions: readonly { value: string; label: string }[];
  modeOptions: readonly AgentCapabilityOption[];
  supportsExecutionPolicies: boolean;
}) {
  return (
    <div class="codex-composer-execution-controls flex min-w-0 flex-wrap items-center gap-1">
      {reasoningEffortOptions.length > 0 && (
        <ComposerOptionDropdown
          label="Thinking"
          value={preferences.reasoningEffort}
          options={reasoningEffortOptions}
          disabled={streaming}
          Icon={Activity}
          onChange={preferenceActions.changeReasoningEffort}
        />
      )}

      {serviceTierOptions.length > 0 && (
        <ComposerOptionDropdown
          label="Speed"
          value={preferences.serviceTier}
          options={serviceTierOptions}
          disabled={streaming}
          Icon={Cpu}
          onChange={preferenceActions.changeServiceTier}
        />
      )}

      {modeOptions.length > 1 && (
        <ComposerOptionDropdown
          label="Mode"
          value={preferences.mode}
          options={modeOptions}
          Icon={MessageSquare}
          onChange={(mode) => {
            const preset = modeOptions.find((option) => option.value === mode);
            preferenceActions.changeMode(mode, preset?.model, preset?.reasoningEffort);
          }}
        />
      )}

      {supportsExecutionPolicies && (
        <ComposerOptionDropdown
          label="Approvals"
          value={preferences.approvalPolicy}
          options={APPROVAL_POLICY_OPTIONS}
          disabled={streaming}
          Icon={ShieldCheck}
          onChange={preferenceActions.changeApprovalPolicy}
        />
      )}

      {supportsExecutionPolicies && (
        <ComposerOptionDropdown
          label="Sandbox"
          value={preferences.sandboxPolicy}
          options={SANDBOX_POLICY_OPTIONS}
          disabled={streaming}
          Icon={Lock}
          onChange={preferenceActions.changeSandboxPolicy}
        />
      )}
    </div>
  );
}

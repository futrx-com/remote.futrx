import { Activity, Cpu, MessageSquare } from "../../primitives/icons";
import { ComposerOptionDropdown } from "./ComposerOptionDropdown";
import type { ComposerPreferenceActions, ComposerPreferences } from "./preferences";

export function ComposerExecutionControls({
  preferences,
  preferenceActions,
  streaming,
  reasoningEffortOptions,
  serviceTierOptions,
  modeOptions,
}: {
  preferences: ComposerPreferences;
  preferenceActions: ComposerPreferenceActions;
  streaming: boolean;
  reasoningEffortOptions: readonly { value: string; label: string }[];
  serviceTierOptions: readonly { value: string; label: string }[];
  modeOptions: readonly { value: string; label: string }[];
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
          onChange={preferenceActions.changeMode}
        />
      )}
    </div>
  );
}

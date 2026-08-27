import type { ChatProvider, SelectedSkill } from "../../../models/chat";
import type { RegisteredSkill } from "../../../models/skill";
import type {
  ComposerModelOption,
  ComposerProviderOption,
} from "../../../state/chat/agentCapabilityState";
import { ComposerAgentPicker } from "./ComposerAgentPicker";
import { SkillPicker } from "./SkillPicker";

export function ComposerAgentControls({
  projectId,
  model,
  provider,
  streaming,
  providerOptions,
  modelOptions,
  modelsLoading,
  modelsRefreshing,
  modelError,
  selectedSkills,
  providerLabel,
  skillsEnabled,
  onSelectSkill,
  onAgentChange,
  onRefreshModels,
}: {
  projectId?: string;
  model: string;
  provider: ChatProvider;
  streaming: boolean;
  providerOptions: readonly ComposerProviderOption[];
  modelOptions: readonly ComposerModelOption[];
  modelsLoading: boolean;
  modelsRefreshing: boolean;
  modelError: string;
  selectedSkills: SelectedSkill[];
  providerLabel: string;
  skillsEnabled: boolean;
  onSelectSkill: (skill: RegisteredSkill) => void;
  onAgentChange: (provider: ChatProvider, model: string) => void;
  onRefreshModels: () => Promise<void>;
}) {
  const selectedCount = selectedSkills.length;
  return (
    <div class="codex-composer-agent-controls flex min-w-0 flex-wrap items-center gap-1">
      <ComposerAgentPicker
        provider={provider}
        model={model}
        streaming={streaming}
        providerOptions={providerOptions}
        modelOptions={modelOptions}
        loading={modelsLoading}
        refreshing={modelsRefreshing}
        error={modelError}
        onChange={onAgentChange}
        onRefresh={onRefreshModels}
      />

      {skillsEnabled && (
        <SkillPicker
          provider={provider}
          providerLabel={providerLabel}
          projectId={projectId}
          selectedCount={selectedCount}
          onSelect={(skill) => onSelectSkill(skill)}
        />
      )}
    </div>
  );
}

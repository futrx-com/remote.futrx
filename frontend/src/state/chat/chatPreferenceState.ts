import type {
  ChatMeta,
  ChatMode,
  ChatProvider,
  ReasoningEffort,
  SelectedSkill,
  ServiceTier,
} from "../../models/chat";
import type { ChatSettings } from "../../models/settings";
import type { RegisteredSkill } from "../../models/skill";

export interface ResolvedChatMeta extends ChatMeta {
  provider: ChatProvider;
  model: string;
  mode: ChatMode;
  reasoningEffort: ReasoningEffort;
  serviceTier: ServiceTier;
}

class ChatPreferenceState {
  resolveMeta(
    chat: ChatMeta,
    loadedMeta: ChatMeta | null,
    defaults: ChatSettings
  ): ResolvedChatMeta {
    const baseMeta = loadedMeta ?? chat;
    const selectedSkills = chat.selectedSkills ?? baseMeta.selectedSkills;
    return {
      ...baseMeta,
      // Workspace chat upserts are the live cross-client source. Prefer their
      // preference fields over the one-time detail fetch so a selection made
      // on another browser is reflected while this chat remains open.
      provider: chat.provider || baseMeta.provider || defaults.provider,
      model: chat.model ?? baseMeta.model ?? defaults.model,
      mode: chat.mode || baseMeta.mode || defaults.mode,
      reasoningEffort:
        chat.reasoningEffort ?? baseMeta.reasoningEffort ?? defaults.reasoningEffort,
      serviceTier: chat.serviceTier ?? baseMeta.serviceTier ?? defaults.serviceTier,
      ...(selectedSkills !== undefined ? { selectedSkills } : {}),
    };
  }

  selectedSkill(skill: RegisteredSkill, defaultProvider: ChatProvider): SelectedSkill {
    return {
      name: skill.name,
      command: skill.command || skill.name,
      provider: skill.provider || defaultProvider,
      source: skill.source,
    };
  }

  includesSkill(
    selectedSkills: SelectedSkill[],
    skill: SelectedSkill,
    defaultProvider: ChatProvider
  ): boolean {
    const key = this.skillKey(skill, defaultProvider);
    return selectedSkills.some((selected) => this.skillKey(selected, defaultProvider) === key);
  }

  withoutSkill(
    selectedSkills: SelectedSkill[],
    skill: SelectedSkill,
    defaultProvider: ChatProvider
  ): SelectedSkill[] {
    const key = this.skillKey(skill, defaultProvider);
    return selectedSkills.filter((selected) => this.skillKey(selected, defaultProvider) !== key);
  }

  private skillKey(
    skill: SelectedSkill | RegisteredSkill,
    defaultProvider: ChatProvider
  ): string {
    const provider = skill.provider || defaultProvider;
    const command = (skill.command || skill.name).trim().toLowerCase();
    // Remote initially advertises this reserved skill from its built-in
    // catalog, then provisions the same skill into the project workspace.
    // Keep its identity stable across that source transition.
    const source = command === "scheduled-tasks" ? "remote" : skill.source || "";
    return `${provider}:${source.toLowerCase()}:${command}`;
  }
}

export const chatPreferenceState = new ChatPreferenceState();

import type {
  ChatMeta,
  ApprovalPolicy,
  ChatMode,
  ChatProvider,
  ReasoningEffort,
  SelectedSkill,
  ServiceTier,
  SandboxPolicy,
} from "../../../models/chat";
import type { RegisteredSkill } from "../../../models/skill";
import { useUserSettingsContext } from "../../context/UserSettingsContext";
import { chatPreferenceState } from "./chatPreferenceState";
import { useChatMetaActions } from "./useChatMetaActions";

export function useChatPreferences({
  chat,
  loadedMeta,
  refreshMeta,
}: {
  chat: ChatMeta;
  loadedMeta: ChatMeta | null;
  refreshMeta: () => Promise<void>;
}) {
  const { settings, setChatSettings } = useUserSettingsContext();
  const preferenceScope = chat.projectId ? "project" : "host";
  const defaults = preferenceScope === "project" ? settings.projectChat : settings.chat;
  const displayMeta = chatPreferenceState.resolveMeta(chat, loadedMeta, defaults);
  const displayProvider = displayMeta.provider;
  const displayModel = displayMeta.model;
  const displayMode = displayMeta.mode;
  const selectedSkills = displayMeta.selectedSkills || [];
  const metaActions = useChatMetaActions({ chatId: chat.id, refreshMeta });

  function changeAgent(provider: ChatProvider, model: string) {
    if (provider === displayProvider && model === displayModel) return;
    const providerChanged = provider !== displayProvider;
    metaActions.applyMeta({
      provider,
      model,
      reasoningEffort: "",
      serviceTier: "",
      ...(providerChanged ? { selectedSkills: [] } : {}),
    });
    void setChatSettings(preferenceScope, {
      provider,
      model,
      reasoningEffort: "",
      serviceTier: "",
    });
  }

  function selectSkill(skill: RegisteredSkill) {
    const next = chatPreferenceState.selectedSkill(skill, displayProvider);
    if (chatPreferenceState.includesSkill(selectedSkills, next, displayProvider)) return;
    metaActions.applyMeta({ selectedSkills: [...selectedSkills, next] });
  }

  function removeSelectedSkill(skill: SelectedSkill) {
    metaActions.applyMeta({
      selectedSkills: chatPreferenceState.withoutSkill(
        selectedSkills,
        skill,
        displayProvider
      ),
    });
  }

  function changeMode(mode: ChatMode, modelPreset?: string, reasoningPreset?: string) {
    const patch = {
      mode,
      ...(modelPreset ? { model: modelPreset } : {}),
      ...(reasoningPreset ? { reasoningEffort: reasoningPreset } : {}),
    };
    metaActions.applyMeta(patch);
    void setChatSettings(preferenceScope, patch);
  }

  function changeReasoningEffort(reasoningEffort: ReasoningEffort) {
    metaActions.applyMeta({ reasoningEffort });
    void setChatSettings(preferenceScope, { reasoningEffort });
  }

  function changeServiceTier(serviceTier: ServiceTier) {
    metaActions.applyMeta({ serviceTier });
    void setChatSettings(preferenceScope, { serviceTier });
  }

  function changeApprovalPolicy(approvalPolicy: ApprovalPolicy) {
    metaActions.applyMeta({ approvalPolicy });
    void setChatSettings(preferenceScope, { approvalPolicy });
  }

  function changeSandboxPolicy(sandboxPolicy: SandboxPolicy) {
    metaActions.applyMeta({ sandboxPolicy });
    void setChatSettings(preferenceScope, { sandboxPolicy });
  }

  return {
    displayMeta,
    displayMode,
    selectedSkills,
    changeAgent,
    changeMode,
    changeReasoningEffort,
    changeServiceTier,
    changeApprovalPolicy,
    changeSandboxPolicy,
    selectSkill,
    removeSelectedSkill,
  };
}

import type { RefObject } from "preact";
import { useState } from "preact/hooks";
import { modelShortLabel, providerDisplayLabel } from "../../../config/chat";
import type { QueuedPrompt, SelectedSkill } from "../../../models/chat";
import type { RegisteredSkill } from "../../../models/skill";
import type { Attachment } from "../../../models/upload";
import { useComposerAgentCapabilities } from "../../../state/hooks/chat/useComposerAgentCapabilities";
import { ChevronDown, Settings } from "../../primitives/icons";
import { AttachmentTray } from "./AttachmentTray";
import { AttachButton } from "./AttachButton";
import { ComposerAgentControls } from "./ComposerAgentControls";
import { ComposerDropOverlay } from "./ComposerDropOverlay";
import { ComposerExecutionControls } from "./ComposerExecutionControls";
import { PromptTextarea } from "./PromptTextarea";
import { QueuedPromptList } from "./QueuedPromptList";
import { SelectedSkillChips } from "./SelectedSkillChips";
import { SendControls } from "./SendControls";
import type { ComposerPreferenceActions, ComposerPreferences } from "./preferences";

export interface ChatComposerProps {
  projectId?: string;
  streaming: boolean;
  canSendPrompt: boolean;
  preferences: ComposerPreferences;
  preferenceActions: ComposerPreferenceActions;
  queuedPrompts: QueuedPrompt[];
  selectedSkills: SelectedSkill[];
  attachments: Attachment[];
  uploading: boolean;
  dragging: boolean;
  text: string;
  textareaRef: RefObject<HTMLTextAreaElement>;
  fileInputRef: RefObject<HTMLInputElement>;
  onTextChange: (text: string) => void;
  onFilesSelected: (files: File[]) => void;
  onPaste: (event: ClipboardEvent) => void;
  onSend: () => void;
  onCancel: () => void;
  onRemoveQueued: (id: string) => void;
  onRemoveAttachment: (id: string) => void;
  onSelectSkill: (skill: RegisteredSkill) => void;
  onRemoveSelectedSkill: (skill: SelectedSkill) => void;
}

export function ChatComposer({
  projectId,
  streaming,
  canSendPrompt,
  preferences,
  preferenceActions,
  queuedPrompts,
  selectedSkills,
  attachments,
  uploading,
  dragging,
  text,
  textareaRef,
  fileInputRef,
  onTextChange,
  onFilesSelected,
  onPaste,
  onSend,
  onCancel,
  onRemoveQueued,
  onRemoveAttachment,
  onSelectSkill,
  onRemoveSelectedSkill,
}: ChatComposerProps) {
  const capabilityState = useComposerAgentCapabilities({
    projectId,
    provider: preferences.provider,
    model: preferences.model,
    mode: preferences.mode,
    reasoningEffort: preferences.reasoningEffort,
    serviceTier: preferences.serviceTier,
    actions: preferenceActions,
  });
  const {
    providerOptions,
    modelOptions,
    reasoningEffortOptions,
    serviceTierOptions,
    modeOptions,
    loading: modelsLoading,
    refreshing,
    error: capabilityError,
    refresh: refreshCapabilities,
  } = capabilityState;
  const [mobileSettingsOpen, setMobileSettingsOpen] = useState(false);
  const disconnected = !canSendPrompt && !streaming;
  const hasContent = text.trim().length > 0 || attachments.some((attachment) => attachment.serverPath);
  const canSend = !uploading && !disconnected && hasContent;
  const providerLabel = providerOptions.find(
    (option) => option.value === preferences.provider,
  )?.label || providerDisplayLabel(preferences.provider);
  const modelLabel = modelOptions.find(
    (option) => option.value === preferences.model,
  )?.label || modelShortLabel(preferences.model);
  const settingsSummary = `${providerLabel} · ${modelLabel}`;
  const skillsEnabled = capabilityState.providerCapabilities?.features?.skills !== "none";

  function toggleMobileSettings() {
    setMobileSettingsOpen((open) => {
      if (!open) textareaRef.current?.blur();
      return !open;
    });
  }

  return (
    <div class="codex-composer-shell flex-none z-20 relative bg-[#0b0d11] border-t border-white/10">
      {dragging && <ComposerDropOverlay />}

      <SelectedSkillChips skills={selectedSkills} onRemove={onRemoveSelectedSkill} />
      <QueuedPromptList queuedPrompts={queuedPrompts} onRemove={onRemoveQueued} />
      <AttachmentTray attachments={attachments} onRemove={onRemoveAttachment} />

      <div class="codex-composer-card mx-3 my-2 overflow-visible rounded-xl border border-white/10 bg-[#15171c] shadow-[0_8px_24px_rgba(0,0,0,0.18)]">
        <form
          onSubmit={(event) => {
            event.preventDefault();
            onSend();
          }}
          class="codex-composer-form composer-form flex gap-1.5 items-end px-2 pt-2"
        >
          <AttachButton
            fileInputRef={fileInputRef}
            uploading={uploading}
            disconnected={disconnected}
            onFilesSelected={onFilesSelected}
          />
          <PromptTextarea
            textareaRef={textareaRef}
            text={text}
            uploading={uploading}
            streaming={streaming}
            disconnected={disconnected}
            onTextChange={onTextChange}
            onPaste={onPaste}
            onSend={onSend}
          />
          <button
            type="button"
            onClick={toggleMobileSettings}
            class={`codex-mobile-settings-trigger min-w-0 items-center gap-1.5 rounded-full border px-3 text-left transition md:hidden
                    ${mobileSettingsOpen ? "border-accent-blue/40 bg-accent-blue/[0.12] text-accent-blue" : "border-white/10 bg-white/[0.045] text-ink-200"}`}
            aria-label={`${mobileSettingsOpen ? "Hide" : "Show"} composer settings. ${settingsSummary}`}
            aria-controls="mobile-composer-settings"
            aria-expanded={mobileSettingsOpen}
          >
            <Settings class="h-3.5 w-3.5 flex-none" aria-hidden="true" />
            <span class="min-w-0 flex-1 truncate text-[12px] font-semibold">{settingsSummary}</span>
            <ChevronDown
              class={`h-3 w-3 flex-none transition-transform ${mobileSettingsOpen ? "rotate-180" : ""}`}
              aria-hidden="true"
            />
          </button>
          <SendControls
            streaming={streaming}
            canSend={canSend}
            disconnected={disconnected}
            onCancel={onCancel}
          />
        </form>

        {mobileSettingsOpen && (
          <div
            id="mobile-composer-settings"
            class="codex-mobile-settings-panel border-t border-white/[0.07] px-2.5 pb-2.5 pt-2 md:hidden"
            role="group"
            aria-label="Composer settings"
          >
            <div class="mb-1.5 text-[10px] font-semibold uppercase tracking-[0.12em] text-ink-400">
              Agent and model
            </div>
            <ComposerAgentControls
              projectId={projectId}
              model={preferences.model}
              provider={preferences.provider}
              streaming={streaming}
              providerOptions={providerOptions}
              modelOptions={modelOptions}
              modelsLoading={modelsLoading}
              modelsRefreshing={refreshing}
              modelError={capabilityError}
              selectedSkills={selectedSkills}
              providerLabel={providerLabel}
              skillsEnabled={skillsEnabled}
              onSelectSkill={onSelectSkill}
              onAgentChange={preferenceActions.changeAgent}
              onRefreshModels={refreshCapabilities}
            />

            <div class="mb-1.5 mt-2.5 text-[10px] font-semibold uppercase tracking-[0.12em] text-ink-400">
              Execution
            </div>
            <ComposerExecutionControls
              preferences={preferences}
              preferenceActions={preferenceActions}
              streaming={streaming}
              reasoningEffortOptions={reasoningEffortOptions}
              serviceTierOptions={serviceTierOptions}
              modeOptions={modeOptions}
            />
          </div>
        )}

        <div class="codex-composer-control-deck hidden min-w-0 flex-wrap items-center justify-between gap-1.5 border-t border-white/[0.07] px-2 py-1.5 md:flex">
          <ComposerAgentControls
            projectId={projectId}
            model={preferences.model}
            provider={preferences.provider}
            streaming={streaming}
            providerOptions={providerOptions}
            modelOptions={modelOptions}
            modelsLoading={modelsLoading}
            modelsRefreshing={refreshing}
            modelError={capabilityError}
            selectedSkills={selectedSkills}
            providerLabel={providerLabel}
            skillsEnabled={skillsEnabled}
            onSelectSkill={onSelectSkill}
            onAgentChange={preferenceActions.changeAgent}
            onRefreshModels={refreshCapabilities}
          />

          <ComposerExecutionControls
            preferences={preferences}
            preferenceActions={preferenceActions}
            streaming={streaming}
            reasoningEffortOptions={reasoningEffortOptions}
            serviceTierOptions={serviceTierOptions}
            modeOptions={modeOptions}
          />
        </div>
      </div>
    </div>
  );
}

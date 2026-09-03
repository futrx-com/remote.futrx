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
  const hasExecutionControls =
    reasoningEffortOptions.length > 0 || serviceTierOptions.length > 0 || modeOptions.length > 1;

  function toggleMobileSettings() {
    setMobileSettingsOpen((open) => {
      if (!open) textareaRef.current?.blur();
      return !open;
    });
  }

  return (
    <div class="codex-composer-shell relative z-20 flex-none bg-canvas">
      {dragging && <ComposerDropOverlay />}

      <SelectedSkillChips skills={selectedSkills} onRemove={onRemoveSelectedSkill} />
      <QueuedPromptList queuedPrompts={queuedPrompts} onRemove={onRemoveQueued} />
      <AttachmentTray attachments={attachments} onRemove={onRemoveAttachment} />

      {/* One card: the prompt gets the full width, controls sit beneath it. */}
      <div class="codex-composer-card mx-3 mb-3 overflow-visible rounded-panel border border-line bg-surface shadow-pop">
        <form
          onSubmit={(event) => {
            event.preventDefault();
            onSend();
          }}
          class="codex-composer-form composer-form flex flex-col px-2.5 pt-2"
        >
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

          <div class="codex-composer-control-deck flex min-w-0 items-center gap-1.5 pt-1.5">
            <AttachButton
              fileInputRef={fileInputRef}
              uploading={uploading}
              disconnected={disconnected}
              onFilesSelected={onFilesSelected}
            />

            <div class="hidden min-w-0 flex-1 items-center gap-1.5 md:flex">
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
              {hasExecutionControls && (
                <>
                  <span class="h-4 w-px flex-none bg-line-strong" aria-hidden="true" />
                  <ComposerExecutionControls
                    preferences={preferences}
                    preferenceActions={preferenceActions}
                    streaming={streaming}
                    reasoningEffortOptions={reasoningEffortOptions}
                    serviceTierOptions={serviceTierOptions}
                    modeOptions={modeOptions}
                  />
                </>
              )}
            </div>

            <button
              type="button"
              onClick={toggleMobileSettings}
              class={`codex-mobile-settings-trigger flex h-8 min-w-0 flex-1 items-center gap-1.5 rounded-control px-2 text-left transition md:hidden
                      ${mobileSettingsOpen ? "bg-accent-blue/[0.14] text-accent-blue" : "text-ink-300 hover:bg-tint-strong"}`}
              aria-label={`${mobileSettingsOpen ? "Hide" : "Show"} composer settings. ${settingsSummary}`}
              aria-controls="mobile-composer-settings"
              aria-expanded={mobileSettingsOpen}
            >
              <Settings class="h-3.5 w-3.5 flex-none" aria-hidden="true" />
              <span class="min-w-0 flex-1 truncate text-[12px] font-medium">{settingsSummary}</span>
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
          </div>
        </form>

        {mobileSettingsOpen && (
          <div
            id="mobile-composer-settings"
            class="codex-mobile-settings-panel border-t border-line px-2.5 pb-2.5 pt-2 md:hidden"
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

            {hasExecutionControls && (
              <>
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
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

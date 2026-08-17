import { useEffect, useRef } from "preact/hooks";
import type { ProjectTemplate } from "../../models/template";
import {
  newProjectState,
  type NewProjectState,
} from "../../state/projects/newProjectState";
import { AlertCircle, Check, Loader, X } from "../primitives/icons";
import { templateIcon } from "./templateIcons";

export function NewProjectDialog({
  state,
  onNameChange,
  onSelectTemplate,
  onSubmit,
  onClose,
}: {
  state: NewProjectState;
  onNameChange: (name: string) => void;
  onSelectTemplate: (template: string) => void;
  onSubmit: () => void;
  onClose: () => void;
}) {
  const nameInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (state.open) nameInput.current?.focus();
  }, [state.open]);

  useEffect(() => {
    if (!state.open) return;
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") onClose();
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [state.open, onClose]);

  if (!state.open) return null;

  const canSubmit = newProjectState.canSubmit(state);
  const notice = newProjectState.firstStartNotice(state);

  return (
    <div
      class="fixed inset-0 z-50 flex items-end md:items-center justify-center bg-black/60 p-0 md:p-4"
      onClick={onClose}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="new-project-title"
        class="w-full md:max-w-2xl max-h-[92vh] flex flex-col rounded-t-xl md:rounded-xl
               border border-white/10 bg-[#101318] shadow-2xl overflow-hidden"
        onClick={(event) => event.stopPropagation()}
      >
        <header class="flex-none flex items-start gap-3 px-4 py-3 border-b border-white/[0.08]">
          <div class="flex-1 min-w-0">
            <h2 id="new-project-title" class="text-[15px] font-semibold text-ink-50">
              New project
            </h2>
            <p class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">
              Each project gets its own container. The template decides what is
              installed inside it and cannot be changed later.
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            class="h-8 w-8 rounded-md text-ink-300 hover:text-ink-50 hover:bg-white/[0.08] grid place-items-center"
            aria-label="Close"
          >
            <X class="w-4 h-4" />
          </button>
        </header>

        <form
          class="flex-1 min-h-0 flex flex-col"
          onSubmit={(event) => {
            event.preventDefault();
            if (canSubmit) onSubmit();
          }}
        >
          <div class="flex-1 min-h-0 overflow-y-auto touch-scroll px-4 py-4 space-y-4">
            <label class="block">
              <span class="text-[12.5px] font-medium text-ink-200">Project name</span>
              <input
                ref={nameInput}
                type="text"
                value={state.name}
                onInput={(event) => onNameChange((event.target as HTMLInputElement).value)}
                placeholder="My project"
                class="mt-1.5 w-full h-10 px-3 rounded-md bg-white/[0.04] border border-white/10
                       text-[14px] text-ink-50 placeholder:text-ink-400
                       focus:outline-none focus:border-accent-blue/60"
              />
            </label>

            <div>
              <div class="text-[12.5px] font-medium text-ink-200">Template</div>
              <TemplatePicker
                templates={state.templates}
                loading={state.templatesLoading}
                error={state.templatesError}
                selected={state.template}
                onSelect={onSelectTemplate}
              />
              {notice && (
                <p class="mt-2 text-[12px] text-ink-300 leading-snug">{notice}</p>
              )}
            </div>

            {state.error && (
              <div class="flex items-start gap-2.5 rounded-md border border-accent-red/30
                          bg-accent-red/[0.08] px-3 py-2 text-[12.5px]">
                <AlertCircle class="w-4 h-4 mt-0.5 flex-none text-accent-red" />
                <span class="text-ink-100 break-words">{state.error}</span>
              </div>
            )}
          </div>

          <footer class="flex-none flex justify-end gap-2 px-4 py-3 border-t border-white/[0.08]">
            <button
              type="button"
              onClick={onClose}
              class="h-10 px-3 rounded-md text-[13.5px] text-ink-200 hover:text-ink-50 hover:bg-white/[0.08]"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={!canSubmit}
              class="h-10 px-4 rounded-md text-[13.5px] font-medium text-white
                     bg-accent-blue hover:bg-accent-blue/90 disabled:opacity-50
                     disabled:cursor-not-allowed inline-flex items-center gap-2"
            >
              {state.submitting && <Loader class="w-4 h-4 animate-spin" />}
              Create project
            </button>
          </footer>
        </form>
      </div>
    </div>
  );
}

function TemplatePicker({
  templates,
  loading,
  error,
  selected,
  onSelect,
}: {
  templates: ProjectTemplate[];
  loading: boolean;
  error: string;
  selected: string;
  onSelect: (template: string) => void;
}) {
  if (loading && templates.length === 0) {
    return (
      <div class="mt-1.5 flex items-center gap-2 text-[12.5px] text-ink-300">
        <Loader class="w-4 h-4 animate-spin" /> Loading templates…
      </div>
    );
  }
  if (error && templates.length === 0) {
    return (
      <div class="mt-1.5 text-[12.5px] text-ink-300">
        Templates could not be loaded ({error}). The project will be created
        from the blank template.
      </div>
    );
  }

  return (
    <div class="mt-1.5 grid grid-cols-1 sm:grid-cols-2 gap-2" role="radiogroup" aria-label="Template">
      {templates.map((template) => {
        const active = template.name === selected;
        const Icon = templateIcon(template.icon);
        return (
          <button
            key={template.name}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onSelect(template.name)}
            class={`text-left rounded-lg border p-3 transition-colors ${
              active
                ? "border-accent-blue/60 bg-accent-blue/[0.08]"
                : "border-white/10 bg-white/[0.03] hover:bg-white/[0.06]"
            }`}
          >
            <div class="flex items-center gap-2">
              <Icon class={`w-4 h-4 flex-none ${active ? "text-accent-blue" : "text-ink-300"}`} />
              <span class="text-[13.5px] font-medium text-ink-50 truncate">{template.title}</span>
              {active && <Check class="w-4 h-4 ml-auto flex-none text-accent-blue" />}
            </div>
            <p class="mt-1 text-[12px] text-ink-300 leading-snug">{template.description}</p>
            <div class="mt-1.5 flex flex-wrap gap-1">
              {template.default && <TemplateTag label="default" />}
              {template.prebuiltImageAvailable && <TemplateTag label="prebuilt image" />}
              {template.defaultPorts?.map((port) => (
                <TemplateTag key={port} label={`:${port}`} mono />
              ))}
            </div>
          </button>
        );
      })}
    </div>
  );
}

function TemplateTag({ label, mono = false }: { label: string; mono?: boolean }) {
  return (
    <span
      class={`inline-flex items-center h-5 px-1.5 rounded bg-white/[0.06] text-[10.5px] text-ink-300 ${
        mono ? "font-mono" : ""
      }`}
    >
      {label}
    </span>
  );
}

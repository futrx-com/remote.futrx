import { useEffect, useRef, useState } from "preact/hooks";
import type { ProjectMeta } from "../../models/project";
import { createProjectForm } from "../../state/projects/createProjectForm";
import { Loader, X } from "../primitives/icons";

const MAX_NAME_LEN = 40;

export function CreateProjectModal({
  open,
  projects,
  onClose,
  onCreate,
}: {
  open: boolean;
  projects: ProjectMeta[];
  onClose: () => void;
  onCreate: (name: string) => Promise<unknown>;
}) {
  const [name, setName] = useState("");
  const [touched, setTouched] = useState(false);
  const [creating, setCreating] = useState(false);
  const [submitError, setSubmitError] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    setName("");
    setTouched(false);
    setCreating(false);
    setSubmitError("");
    // Focus after the pop animation has started so the browser doesn't
    // scroll a half-positioned card into view.
    const timer = setTimeout(() => inputRef.current?.focus(), 60);
    return () => clearTimeout(timer);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  if (!open) return null;

  // The create request only resolves once the container is provisioned, and
  // the live project list picks up the in-flight project before that. Freeze
  // validation while creating so our own project isn't flagged as a duplicate.
  const validation = createProjectForm.validate(name, projects);
  const slug = validation.slug;
  const showError = !creating && (!!submitError || (touched && !validation.ok && !!validation.message));
  const hint = creating
    ? "Provisioning container…"
    : submitError
      || (touched && !validation.ok ? validation.message : "")
      || validation.message
      || "Lowercase letters, numbers and dashes.";
  const canSubmit = validation.ok && !creating;

  function close() {
    if (!creating) onClose();
  }

  async function submit() {
    if (!canSubmit) return;
    setCreating(true);
    setSubmitError("");
    try {
      await onCreate(name.trim());
      onClose();
    } catch (error) {
      setSubmitError("Create failed: " + (error as Error).message);
      setCreating(false);
    }
  }

  return (
    <div class="fixed inset-0 z-50 flex items-center justify-center p-4 sm:p-8">
      <div
        class="absolute inset-0 bg-black/70 backdrop-blur-[3px] modal-backdrop-fade"
        onClick={close}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="create-project-title"
        class="theme-menu-surface modal-card-pop relative w-full max-w-[480px] overflow-hidden rounded-[14px] border border-white/10 bg-ink-800 text-ink-50 shadow-[0_24px_64px_rgba(0,0,0,.6)]"
      >
        <div class="flex items-start justify-between gap-4 px-5 pb-3.5 pt-[18px]">
          <div class="flex flex-col gap-[3px]">
            <div id="create-project-title" class="text-[15px] font-semibold tracking-[-0.01em]">
              New project
            </div>
            <div class="text-[12.5px] text-ink-300">
              Creates a workspace container and clones nothing yet.
            </div>
          </div>
          <button
            onClick={close}
            aria-label="Close"
            class="flex h-7 w-7 shrink-0 items-center justify-center rounded-[7px] text-ink-300 transition-colors hover:bg-white/5 hover:text-ink-100"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <div class="flex flex-col gap-3.5 border-t border-white/10 p-5">
          <div class="flex flex-col gap-[7px]">
            <label
              for="create-project-name"
              class="text-xs uppercase tracking-[0.08em] text-ink-300"
            >
              Project name
            </label>
            <input
              id="create-project-name"
              ref={inputRef}
              value={name}
              onInput={(event) => {
                setName((event.target as HTMLInputElement).value.slice(0, MAX_NAME_LEN));
                setTouched(true);
                setSubmitError("");
              }}
              onKeyDown={(event) => {
                if (event.key === "Enter") void submit();
              }}
              placeholder="e.g. futrx-web"
              autocomplete="off"
              spellcheck={false}
              maxLength={MAX_NAME_LEN}
              disabled={creating}
              class={`theme-submenu-surface w-full rounded-[9px] border bg-[#101116] px-3 py-2.5 font-mono text-sm text-ink-100 outline-none transition-[border-color,box-shadow] duration-150 ${
                showError
                  ? "border-accent-red/60 shadow-[0_0_0_3px_rgba(255,123,114,.12)]"
                  : touched && (validation.ok || creating)
                    ? "border-white/[0.12] shadow-[0_0_0_3px_rgba(138,180,255,.14)]"
                    : "border-white/[0.12]"
              }`}
            />
            <div class="flex min-h-[18px] items-center justify-between gap-3">
              <div class={`text-xs ${showError ? "text-accent-red" : "text-ink-400"}`}>{hint}</div>
              <div class="text-[11.5px] tabular-nums text-ink-400">
                {name ? `${name.trim().length}/${createProjectForm.maxSlugLen}` : ""}
              </div>
            </div>
          </div>

          <div class="flex flex-col gap-2 rounded-[10px] border border-white/10 bg-white/[0.03] px-3.5 py-3">
            <div class="flex items-baseline justify-between gap-3">
              <span class="text-xs text-ink-300">Container</span>
              <span class="font-mono text-[12.5px] text-ink-200">{slug || "—"}</span>
            </div>
            <div class="flex items-baseline justify-between gap-3">
              <span class="text-xs text-ink-300">Path</span>
              <span class="truncate font-mono text-[12.5px] text-ink-200">
                {createProjectForm.pathPreview(projects, slug)}
              </span>
            </div>
          </div>
        </div>

        <div class="flex items-center justify-end gap-2 border-t border-white/10 bg-white/[0.02] px-5 py-3.5">
          <button
            onClick={close}
            class="rounded-lg border border-white/[0.12] px-3.5 py-2 text-[13px] text-ink-200 transition-colors hover:bg-white/5 hover:text-ink-100"
          >
            Cancel
          </button>
          <button
            onClick={() => void submit()}
            disabled={!canSubmit}
            class={`inline-flex items-center gap-[7px] rounded-lg border border-transparent px-[15px] py-2 text-[13px] font-medium transition-colors ${
              validation.ok
                ? "bg-accent-blue text-ink-900"
                : "cursor-not-allowed bg-white/[0.07] text-ink-400"
            } ${creating ? "opacity-80" : ""}`}
          >
            {creating && <Loader class="h-3.5 w-3.5 animate-spin" />}
            {creating ? "Creating…" : "Create project"}
          </button>
        </div>
      </div>
    </div>
  );
}

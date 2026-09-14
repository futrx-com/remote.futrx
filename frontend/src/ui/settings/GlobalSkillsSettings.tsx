import { useState } from "preact/hooks";
import type { GlobalSkill } from "../../models/globalSkill";
import type { ProjectMeta } from "../../models/project";
import type { GlobalSkillLibrary } from "../../state/hooks/settings/useGlobalSkills";
import {
  globalSkillsState,
  type GlobalSkillDraft,
} from "../../state/settings/globalSkillsState";
import { AlertCircle, Check, Download, Edit, Loader, Plus, Trash, X } from "../primitives/icons";

// GlobalSkillsSettings is the admin surface for the platform-wide skills
// library: every project sees these skills in its picker on top of its own
// workspace skills, and an "always on" skill is preselected in every new chat.
export function GlobalSkillsSettings({
  isAdmin,
  library,
  projects,
}: {
  isAdmin: boolean;
  library: GlobalSkillLibrary;
  projects: ProjectMeta[];
}) {
  const [draft, setDraft] = useState<GlobalSkillDraft | null>(null);
  const [editing, setEditing] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [importOpen, setImportOpen] = useState(false);

  if (!isAdmin) {
    return (
      <section class="rounded-lg border border-white/10 bg-[#101318] p-4 text-[13px] leading-relaxed text-ink-300">
        Global skills are managed by server administrators. Skills published
        here appear in every project's skill picker.
      </section>
    );
  }

  const startCreate = () => {
    setEditing(null);
    setFormError(null);
    setDraft(globalSkillsState.emptyDraft());
  };

  const startEdit = async (skill: GlobalSkill) => {
    setBusy(true);
    setFormError(null);
    try {
      const full = await library.read(skill.name);
      setEditing(skill.name);
      setDraft(globalSkillsState.draftFromSkill(full));
    } catch (cause) {
      setFormError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const submit = async () => {
    if (!draft) return;
    const problem = globalSkillsState.validateDraft(draft);
    if (problem) {
      setFormError(problem);
      return;
    }
    setBusy(true);
    setFormError(null);
    try {
      if (editing) await library.save(editing, draft);
      else await library.create(draft);
      setDraft(null);
      setEditing(null);
    } catch (cause) {
      setFormError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const removeSkill = async (name: string) => {
    if (!confirm(`Delete the global skill "${name}"? Projects lose it on their next chat.`)) {
      return;
    }
    setBusy(true);
    try {
      await library.remove(name);
    } catch (cause) {
      setFormError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <section class="space-y-4">
      <div class="rounded-lg border border-white/10 bg-[#101318] overflow-hidden">
        <header class="px-4 py-3 flex items-start gap-3 border-b border-white/[0.06]">
          <div class="flex-1 min-w-0">
            <div class="text-[14.5px] font-semibold text-ink-50">Global skills</div>
            <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">
              Published to every project alongside its own workspace skills. A
              project skill with the same name always wins; the global copy is
              then shown as shadowed.
            </div>
          </div>
          {library.loading && <Loader class="w-4 h-4 mt-2 text-ink-300 animate-spin" />}
        </header>

        <div class="p-3 space-y-3">
          {(library.error || formError) && (
            <div class="flex items-start gap-2 rounded-md border border-red-500/30 bg-red-500/10 px-3 py-2 text-[12.5px] text-red-200">
              <AlertCircle class="w-4 h-4 flex-none mt-0.5" />
              <span>{formError || library.error}</span>
            </div>
          )}

          <div class="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={startCreate}
              disabled={busy}
              class="inline-flex h-8 items-center gap-1.5 rounded-md bg-accent-blue/[0.14] px-3 text-[12.5px]
                     font-medium text-accent-blue hover:bg-accent-blue/[0.2] disabled:opacity-50"
            >
              <Plus class="w-3.5 h-3.5" /> New skill
            </button>
            <button
              type="button"
              onClick={() => setImportOpen((open) => !open)}
              disabled={busy}
              class="inline-flex h-8 items-center gap-1.5 rounded-md bg-white/[0.06] px-3 text-[12.5px]
                     font-medium text-ink-200 hover:bg-white/[0.1] disabled:opacity-50"
            >
              <Download class="w-3.5 h-3.5" /> Import from project
            </button>
          </div>

          {importOpen && (
            <ImportFromProject
              projects={projects}
              busy={busy}
              onCancel={() => setImportOpen(false)}
              onImport={async (projectId, skill, name) => {
                setBusy(true);
                setFormError(null);
                try {
                  await library.importFromProject(projectId, skill, name);
                  setImportOpen(false);
                } catch (cause) {
                  setFormError((cause as Error).message);
                } finally {
                  setBusy(false);
                }
              }}
            />
          )}

          {library.skills === null ? (
            <div class="px-1 py-2 text-[12.5px] text-ink-400">Loading global skills...</div>
          ) : library.skills.length === 0 ? (
            <div class="px-1 py-2 text-[12.5px] text-ink-400">
              No global skills yet. Create one, or import an existing project skill.
            </div>
          ) : (
            <ul class="space-y-1.5">
              {library.skills.map((skill) => (
                <li
                  key={skill.name}
                  class="flex items-start gap-3 rounded-md border border-white/[0.06] bg-[#0f1217] px-3 py-2.5"
                >
                  <div class="min-w-0 flex-1">
                    <div class="flex flex-wrap items-center gap-2">
                      <span class="text-[13px] font-medium text-ink-100">
                        {skill.title || skill.name}
                      </span>
                      <span class="rounded bg-white/[0.08] px-1.5 py-0.5 text-[10px] uppercase text-ink-400">
                        {skill.name}
                      </span>
                      {skill.alwaysOn && (
                        <span class="rounded bg-accent-blue/[0.16] px-1.5 py-0.5 text-[10px] uppercase text-accent-blue">
                          Always on
                        </span>
                      )}
                    </div>
                    {skill.description && (
                      <p class="mt-1 text-[12px] leading-4 text-ink-400">{skill.description}</p>
                    )}
                    {skill.fileNames && skill.fileNames.length > 1 && (
                      <p class="mt-1 text-[11px] text-ink-500">
                        {skill.fileNames.length} files: {skill.fileNames.join(", ")}
                      </p>
                    )}
                  </div>
                  <div class="flex flex-none items-center gap-1">
                    <button
                      type="button"
                      onClick={() => void library.setAlwaysOn(skill.name, !skill.alwaysOn)}
                      disabled={busy}
                      title={skill.alwaysOn ? "Stop auto-selecting in new chats" : "Auto-select in every new chat"}
                      class={`inline-flex h-7 items-center gap-1 rounded px-2 text-[11.5px] disabled:opacity-50 ${
                        skill.alwaysOn
                          ? "bg-accent-blue/[0.16] text-accent-blue hover:bg-accent-blue/[0.22]"
                          : "text-ink-400 hover:bg-white/[0.08] hover:text-ink-100"
                      }`}
                    >
                      <Check class="w-3.5 h-3.5" /> Always on
                    </button>
                    <button
                      type="button"
                      onClick={() => void startEdit(skill)}
                      disabled={busy}
                      class="inline-flex h-7 w-7 items-center justify-center rounded text-ink-400 hover:bg-white/[0.08] hover:text-ink-100 disabled:opacity-50"
                      title={`Edit ${skill.name}`}
                      aria-label={`Edit ${skill.name}`}
                    >
                      <Edit class="w-3.5 h-3.5" />
                    </button>
                    <button
                      type="button"
                      onClick={() => void removeSkill(skill.name)}
                      disabled={busy}
                      class="inline-flex h-7 w-7 items-center justify-center rounded text-ink-400 hover:bg-red-500/15 hover:text-red-300 disabled:opacity-50"
                      title={`Delete ${skill.name}`}
                      aria-label={`Delete ${skill.name}`}
                    >
                      <Trash class="w-3.5 h-3.5" />
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>

      {draft && (
        <SkillEditor
          draft={draft}
          editing={editing}
          busy={busy}
          onChange={setDraft}
          onCancel={() => {
            setDraft(null);
            setEditing(null);
            setFormError(null);
          }}
          onSubmit={() => void submit()}
        />
      )}
    </section>
  );
}

function SkillEditor({
  draft,
  editing,
  busy,
  onChange,
  onCancel,
  onSubmit,
}: {
  draft: GlobalSkillDraft;
  editing: string | null;
  busy: boolean;
  onChange: (draft: GlobalSkillDraft) => void;
  onCancel: () => void;
  onSubmit: () => void;
}) {
  const [newFilePath, setNewFilePath] = useState("");
  const metadata = globalSkillsState.parseManifest(draft.manifest);
  const extraPaths = Object.keys(draft.extraFiles).sort();

  return (
    <div class="rounded-lg border border-white/10 bg-[#101318] overflow-hidden">
      <header class="px-4 py-3 border-b border-white/[0.06]">
        <div class="text-[14.5px] font-semibold text-ink-50">
          {editing ? `Edit ${editing}` : "New global skill"}
        </div>
        <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">
          SKILL.md needs YAML frontmatter with <code>name</code> and{" "}
          <code>description</code>. The directory name is what the agent
          triggers.
        </div>
      </header>

      <div class="p-3 space-y-3">
        <label class="block">
          <span class="text-[12px] text-ink-300">Directory name</span>
          <div class="mt-1 flex gap-2">
            <input
              value={draft.name}
              disabled={editing !== null}
              onInput={(event) =>
                onChange({
                  ...draft,
                  name: globalSkillsState.normalizeName(
                    (event.currentTarget as HTMLInputElement).value
                  ),
                })
              }
              placeholder="code-review-guard"
              class="h-9 flex-1 rounded-md border border-white/10 bg-[#0b0d11] px-2.5 text-[13px]
                     text-ink-100 placeholder:text-ink-500 focus:outline-none disabled:opacity-60"
            />
            {editing === null && metadata.name && (
              <button
                type="button"
                onClick={() =>
                  onChange({ ...draft, name: globalSkillsState.suggestName(metadata.name) })
                }
                class="h-9 rounded-md bg-white/[0.06] px-3 text-[12px] text-ink-200 hover:bg-white/[0.1]"
              >
                Use "{globalSkillsState.suggestName(metadata.name)}"
              </button>
            )}
          </div>
        </label>

        <label class="block">
          <span class="text-[12px] text-ink-300">SKILL.md</span>
          <textarea
            value={draft.manifest}
            onInput={(event) =>
              onChange({ ...draft, manifest: (event.currentTarget as HTMLTextAreaElement).value })
            }
            rows={16}
            spellcheck={false}
            class="mt-1 w-full rounded-md border border-white/10 bg-[#0b0d11] p-2.5 font-mono text-[12.5px]
                   leading-5 text-ink-100 focus:outline-none"
          />
        </label>

        {extraPaths.map((path) => (
          <label key={path} class="block">
            <span class="flex items-center justify-between text-[12px] text-ink-300">
              <span class="font-mono">{path}</span>
              <button
                type="button"
                onClick={() => {
                  const next = { ...draft.extraFiles };
                  delete next[path];
                  onChange({ ...draft, extraFiles: next });
                }}
                class="inline-flex h-6 w-6 items-center justify-center rounded text-ink-400 hover:bg-white/[0.08] hover:text-ink-100"
                aria-label={`Remove ${path}`}
              >
                <X class="w-3 h-3" />
              </button>
            </span>
            <textarea
              value={draft.extraFiles[path]}
              onInput={(event) =>
                onChange({
                  ...draft,
                  extraFiles: {
                    ...draft.extraFiles,
                    [path]: (event.currentTarget as HTMLTextAreaElement).value,
                  },
                })
              }
              rows={6}
              spellcheck={false}
              class="mt-1 w-full rounded-md border border-white/10 bg-[#0b0d11] p-2.5 font-mono text-[12.5px]
                     leading-5 text-ink-100 focus:outline-none"
            />
          </label>
        ))}

        <div class="flex gap-2">
          <input
            value={newFilePath}
            onInput={(event) => setNewFilePath((event.currentTarget as HTMLInputElement).value)}
            placeholder="references/checklist.md"
            class="h-9 flex-1 rounded-md border border-white/10 bg-[#0b0d11] px-2.5 text-[13px]
                   text-ink-100 placeholder:text-ink-500 focus:outline-none"
          />
          <button
            type="button"
            disabled={!globalSkillsState.isValidFilePath(newFilePath)}
            onClick={() => {
              onChange({
                ...draft,
                extraFiles: { ...draft.extraFiles, [newFilePath.trim()]: "" },
              });
              setNewFilePath("");
            }}
            class="h-9 rounded-md bg-white/[0.06] px-3 text-[12.5px] text-ink-200 hover:bg-white/[0.1] disabled:opacity-40"
          >
            Add file
          </button>
        </div>

        <label class="flex items-center gap-2 text-[12.5px] text-ink-200">
          <input
            type="checkbox"
            checked={draft.alwaysOn}
            onChange={(event) =>
              onChange({ ...draft, alwaysOn: (event.currentTarget as HTMLInputElement).checked })
            }
          />
          Always on — preselect this skill in every new chat
        </label>

        <div class="flex gap-2 pt-1">
          <button
            type="button"
            onClick={onSubmit}
            disabled={busy}
            class="inline-flex h-9 items-center gap-1.5 rounded-md bg-accent-blue/[0.16] px-3.5 text-[13px]
                   font-medium text-accent-blue hover:bg-accent-blue/[0.22] disabled:opacity-50"
          >
            {busy && <Loader class="w-3.5 h-3.5 animate-spin" />}
            {editing ? "Save changes" : "Create skill"}
          </button>
          <button
            type="button"
            onClick={onCancel}
            disabled={busy}
            class="h-9 rounded-md px-3.5 text-[13px] text-ink-300 hover:bg-white/[0.06] disabled:opacity-50"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}

function ImportFromProject({
  projects,
  busy,
  onCancel,
  onImport,
}: {
  projects: ProjectMeta[];
  busy: boolean;
  onCancel: () => void;
  onImport: (projectId: string, skill: string, name?: string) => Promise<void>;
}) {
  const [projectId, setProjectId] = useState(projects[0]?.id ?? "");
  const [skill, setSkill] = useState("");
  const [name, setName] = useState("");

  return (
    <div class="rounded-md border border-white/[0.08] bg-[#0f1217] p-3 space-y-2">
      <div class="text-[12.5px] text-ink-300">
        Copies <code>.agents/skills/&lt;skill&gt;</code> from a project workspace
        into the global library. The project keeps its own copy.
      </div>
      <div class="flex flex-wrap gap-2">
        <select
          value={projectId}
          onChange={(event) => setProjectId((event.currentTarget as HTMLSelectElement).value)}
          class="h-9 min-w-[10rem] rounded-md border border-white/10 bg-[#0b0d11] px-2 text-[13px] text-ink-100"
        >
          {projects.length === 0 && <option value="">No projects</option>}
          {projects.map((project) => (
            <option key={project.id} value={project.id}>
              {project.name}
            </option>
          ))}
        </select>
        <input
          value={skill}
          onInput={(event) => setSkill((event.currentTarget as HTMLInputElement).value)}
          placeholder="project skill name"
          class="h-9 flex-1 min-w-[10rem] rounded-md border border-white/10 bg-[#0b0d11] px-2.5 text-[13px]
                 text-ink-100 placeholder:text-ink-500 focus:outline-none"
        />
        <input
          value={name}
          onInput={(event) => setName((event.currentTarget as HTMLInputElement).value)}
          placeholder="global name (optional)"
          class="h-9 flex-1 min-w-[10rem] rounded-md border border-white/10 bg-[#0b0d11] px-2.5 text-[13px]
                 text-ink-100 placeholder:text-ink-500 focus:outline-none"
        />
      </div>
      <div class="flex gap-2">
        <button
          type="button"
          disabled={busy || !projectId || !skill.trim()}
          onClick={() => void onImport(projectId, skill.trim(), name.trim() || undefined)}
          class="h-8 rounded-md bg-accent-blue/[0.16] px-3 text-[12.5px] font-medium text-accent-blue
                 hover:bg-accent-blue/[0.22] disabled:opacity-40"
        >
          Import
        </button>
        <button
          type="button"
          onClick={onCancel}
          class="h-8 rounded-md px-3 text-[12.5px] text-ink-300 hover:bg-white/[0.06]"
        >
          Cancel
        </button>
      </div>
    </div>
  );
}

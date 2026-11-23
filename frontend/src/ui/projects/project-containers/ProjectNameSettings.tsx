import { useEffect, useState } from "preact/hooks";
import type { ProjectMeta } from "../../../models/project";
import { Loader } from "../../primitives/icons";

// Renames only the display name. The slug, and with it the container and the
// workspace path, stays as it was created.
export function ProjectNameSettings({
  project,
  canRename,
  onRename,
}: {
  project: ProjectMeta;
  canRename: boolean;
  onRename: (name: string) => Promise<void>;
}) {
  const [name, setName] = useState(project.name);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();

  useEffect(() => {
    setName(project.name);
    setError(undefined);
  }, [project.id, project.name]);

  const trimmed = name.trim();
  const validationError = trimmed.includes("--")
    ? "Project name must not contain consecutive hyphens (--)."
    : undefined;
  const unchanged = trimmed === project.name;

  const submit = async (event: Event) => {
    event.preventDefault();
    if (!trimmed || unchanged || validationError) return;
    setSaving(true);
    setError(undefined);
    try {
      await onRename(trimmed);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setSaving(false);
    }
  };

  if (!canRename) {
    return (
      <div class="rounded-md border border-line bg-tint px-3 py-2.5 text-[12.5px] text-ink-300">
        You do not have permission to rename this project.
      </div>
    );
  }

  return (
    <form class="space-y-2" onSubmit={submit}>
      <div class="flex flex-col gap-2 sm:flex-row">
        <input
          type="text"
          value={name}
          onInput={(event) => setName(event.currentTarget.value)}
          aria-label="Project name"
          spellcheck={false}
          class="h-10 flex-1 min-w-0 rounded-md border border-line bg-inset px-3 text-[13px] text-ink-50 outline-none focus:border-accent-blue/60 focus:ring-1 focus:ring-accent-blue/25"
        />
        <button
          type="submit"
          disabled={!trimmed || unchanged || !!validationError || saving}
          class="btn btn-primary btn-sm text-[13px] font-medium disabled:opacity-50 inline-flex items-center justify-center gap-2"
        >
          {saving && <Loader class="w-3.5 h-3.5 animate-spin" />}
          Save
        </button>
      </div>
      <div class="text-[12px] text-ink-400">
        The container and workspace keep the name <code class="font-mono">{project.slug}</code>.
      </div>
      {(validationError || error) && (
        <div class="rounded-md border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2 text-[12.5px] text-accent-red">
          {validationError || error}
        </div>
      )}
    </form>
  );
}

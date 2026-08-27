import { useState } from "preact/hooks";
import type { ProjectMeta } from "../../../models/project";
import { AlertCircle } from "../../primitives/icons";
import { DeleteProjectModal } from "../DeleteProjectModal";

export function ProjectActions({
  project,
  onStart,
  onStop,
  onRestart,
  onDelete,
}: {
  project: ProjectMeta;
  onStart: () => Promise<void>;
  onStop: () => Promise<void>;
  onRestart: () => Promise<void>;
  onDelete: () => Promise<void>;
}) {
  const [busy, setBusy] = useState<"start" | "stop" | "restart" | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const canStart = project.status === "stopped" || project.status === "missing" || project.status === "error";
  const canStop = project.status === "running";
  const canRestart = project.status === "running" || project.status === "error";

  async function run(action: "start" | "stop" | "restart", operation: () => Promise<void>) {
    if (action === "restart" && !confirm(`Force-restart "${project.name}"? All processes inside the container are killed immediately — use this to recover a workspace stuck at its resource limits.`)) return;
    setBusy(action);
    setErr(null);
    try {
      await operation();
    } catch (error) {
      setErr((error as Error).message);
    } finally {
      setBusy(null);
    }
  }

  return (
    <div class="space-y-3">
      <DeleteProjectModal
        open={deleteOpen}
        projectName={project.name}
        onClose={() => setDeleteOpen(false)}
        onDelete={onDelete}
      />
      {err && (
        <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
          <AlertCircle class="w-4 h-4 mt-0.5 flex-none text-accent-red" />
          <div class="text-accent-red break-words">{err}</div>
        </div>
      )}
      <div class="grid gap-2 sm:grid-cols-2">
        <button
          type="button"
          onClick={() => void run("start", onStart)}
          disabled={!canStart || busy !== null}
          class="h-10 rounded-md border border-white/10 bg-white/[0.04] px-3 text-[13px] font-medium text-ink-100 hover:bg-white/[0.08] disabled:opacity-45 disabled:cursor-not-allowed"
        >
          {busy === "start" ? "Starting..." : "Start project"}
        </button>
        <button
          type="button"
          onClick={() => void run("stop", onStop)}
          disabled={!canStop || busy !== null}
          class="h-10 rounded-md border border-white/10 bg-white/[0.04] px-3 text-[13px] font-medium text-ink-100 hover:bg-white/[0.08] disabled:opacity-45 disabled:cursor-not-allowed"
        >
          {busy === "stop" ? "Stopping..." : "Stop project"}
        </button>
      </div>
      <button
        type="button"
        onClick={() => void run("restart", onRestart)}
        disabled={!canRestart || busy !== null}
        title="Host-side kill + fresh boot. Works even when the workspace is unresponsive at its resource limits."
        class="h-10 w-full rounded-md border border-white/10 bg-white/[0.04] px-3 text-[13px] font-medium text-ink-100 hover:bg-white/[0.08] disabled:opacity-45 disabled:cursor-not-allowed"
      >
        {busy === "restart" ? "Restarting..." : "Force restart"}
      </button>
      <button
        type="button"
        onClick={() => setDeleteOpen(true)}
        disabled={busy !== null}
        class="h-10 w-full rounded-md border border-accent-red/30 bg-accent-red/[0.08] px-3 text-[13px] font-semibold text-accent-red hover:bg-accent-red/[0.14] disabled:opacity-45 disabled:cursor-not-allowed"
      >
        Delete project
      </button>
    </div>
  );
}

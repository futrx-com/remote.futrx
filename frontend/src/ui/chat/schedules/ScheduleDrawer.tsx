import type { ComponentChildren } from "preact";
import { useEffect, useMemo, useRef, useState } from "preact/hooks";
import { useConfirm } from "../../../state/context/ConfirmContext";
import { chatApi } from "../../../api/chatApi";
import type { ScheduledTask, UpdateScheduledTaskInput } from "../../../models/schedule";
import {
  AlertCircle,
  CalendarClock,
  Edit,
  Loader,
  Pause,
  Play,
  RotateCcw,
  Trash,
  X,
} from "../../primitives/icons";
import {
  canResumeScheduledTask,
  formatTimestamp,
  isAwaitingArm,
  scheduleDefinition,
  scheduleRunCount,
  sortScheduledTasks,
  toggleActionLabel,
} from "./scheduledTaskView";

type TaskAction = "toggle" | "run" | "delete" | "save";

export function ScheduleDrawer({
  chatId,
  open,
  onClose,
}: {
  chatId: string;
  open: boolean;
  onClose: () => void;
}) {
  const [tasks, setTasks] = useState<ScheduledTask[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState<{ id: string; action: TaskAction } | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);
  const confirm = useConfirm();
  const requestSequence = useRef(0);
  const sortedTasks = useMemo(() => sortScheduledTasks(tasks), [tasks]);
  const enabledCount = tasks.filter((task) => task.enabled).length;

  useEffect(() => {
    requestSequence.current += 1;
    setTasks([]);
    setError(null);
    setNotice(null);
    setBusy(null);
    setEditingId(null);
  }, [chatId]);

  useEffect(() => {
    if (!open) {
      requestSequence.current += 1;
      setNotice(null);
      return;
    }
    void load();
    return () => {
      requestSequence.current += 1;
    };
  }, [chatId, open]);

  async function load() {
    const sequence = ++requestSequence.current;
    setLoading(true);
    setError(null);
    try {
      const response = await chatApi.fetchSchedules(chatId);
      if (sequence !== requestSequence.current) return;
      setTasks(response);
    } catch (err) {
      if (sequence !== requestSequence.current) return;
      setError(errorMessage(err));
    } finally {
      if (sequence === requestSequence.current) setLoading(false);
    }
  }

  async function toggle(task: ScheduledTask) {
    await perform(task, "toggle", async () => {
      await chatApi.updateSchedule(task.id, { enabled: !task.enabled });
      if (!task.enabled && isAwaitingArm(task)) {
        setNotice(`Armed “${task.name}” — it is now on the clock.`);
      }
    });
  }

  async function saveEdit(task: ScheduledTask, changes: UpdateScheduledTaskInput) {
    await perform(task, "save", async () => {
      await chatApi.updateSchedule(task.id, changes);
      setEditingId(null);
      setNotice(`Updated “${changes.name ?? task.name}”.`);
    });
  }

  async function runNow(task: ScheduledTask) {
    await perform(task, "run", async () => {
      await chatApi.runSchedule(task.id);
      setNotice(`Run requested for “${task.name}”.`);
    });
  }

  async function remove(task: ScheduledTask) {
    const confirmed = await confirm({
      title: "Delete scheduled task",
      description: "This action cannot be undone.",
      message: `“${task.name}” and its run history will be permanently removed.`,
      confirmLabel: "Delete task",
    });
    if (!confirmed) return;
    await perform(task, "delete", async () => {
      await chatApi.deleteSchedule(task.id);
      setNotice(`Deleted “${task.name}”.`);
    });
  }

  async function perform(
    task: ScheduledTask,
    action: TaskAction,
    mutation: () => Promise<void>
  ) {
    if (busy) return;
    setBusy({ id: task.id, action });
    setError(null);
    setNotice(null);
    try {
      await mutation();
      await load();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(null);
    }
  }

  const subtitle = loading && tasks.length === 0
    ? "Loading…"
    : tasks.length === 0
      ? "No tasks"
      : `${enabledCount} active · ${tasks.length} total`;

  return (
    <aside
      id="workspace-schedules-pane"
      class={`workspace-pane workspace-schedules-pane relative z-20 h-full flex-none overflow-hidden bg-surface border-l border-line shadow-2xl
              transition-[width,opacity] duration-200 ease-out ${open ? "opacity-100" : "opacity-0 border-l-0 pointer-events-none"}`}
      aria-hidden={!open}
      aria-label="Scheduled tasks"
    >
      <div
        class={`h-full min-h-0 w-full flex flex-col transition-transform duration-200 ease-out
                ${open ? "translate-x-0" : "translate-x-full"}`}
      >
        <header class="workspace-pane-header codex-header flex-none bg-surface border-b border-line px-3 md:px-4 pb-2.5 flex items-center gap-2">
          <div class="h-9 w-9 rounded-md bg-tint border border-line grid place-items-center flex-none">
            <CalendarClock class="w-4 h-4 text-accent-blue" />
          </div>
          <div class="min-w-0 flex-1">
            <h2 class="truncate text-[15px] md:text-base font-semibold text-ink-50">
              Scheduled tasks
            </h2>
            <div class="truncate text-[12px] text-ink-300">{subtitle}</div>
          </div>
          <button
            type="button"
            onClick={() => void load()}
            disabled={loading || !!busy}
            class="h-9 w-9 rounded-md bg-tint hover:bg-tint-strong border border-line text-ink-200 grid place-items-center disabled:opacity-50"
            title="Refresh scheduled tasks"
            aria-label="Refresh scheduled tasks"
          >
            {loading ? <Loader class="w-4 h-4 animate-spin" /> : <RotateCcw class="w-4 h-4" />}
          </button>
          <button
            type="button"
            onClick={onClose}
            class="h-9 w-9 rounded-md bg-tint hover:bg-tint-strong border border-line text-ink-200 grid place-items-center"
            title="Close scheduled tasks"
            aria-label="Close scheduled tasks"
            data-workspace-pane-close
          >
            <X class="w-4 h-4" />
          </button>
        </header>

        <div class="flex-1 min-h-0 overflow-y-auto touch-scroll px-3 md:px-4 py-3">
          {error && (
            <div class="mb-3 flex items-start gap-2.5 rounded-md border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
              <AlertCircle class="mt-0.5 h-4 w-4 flex-none text-accent-red" />
              <div class="min-w-0 flex-1 break-words text-accent-red">{error}</div>
              <button
                type="button"
                onClick={() => void load()}
                class="flex-none text-[12px] text-ink-200 underline decoration-ink-400/60 underline-offset-2 hover:text-white"
              >
                Retry
              </button>
            </div>
          )}
          {notice && (
            <div class="mb-3 rounded-md border border-accent-green/25 bg-accent-green/[0.08] px-3 py-2 text-[12.5px] text-accent-green">
              {notice}
            </div>
          )}

          {loading && tasks.length === 0 ? (
            <div class="h-36 grid place-items-center text-[13px] text-ink-300">
              <div class="flex items-center gap-2">
                <Loader class="w-4 h-4 animate-spin" />
                Loading scheduled tasks…
              </div>
            </div>
          ) : tasks.length === 0 && !error ? (
            <EmptyScheduleState />
          ) : (
            <div class="space-y-2.5">
              {sortedTasks.map((task) => (
                <ScheduledTaskCard
                  key={task.id}
                  task={task}
                  busyAction={busy?.id === task.id ? busy.action : null}
                  actionsDisabled={!!busy}
                  editing={editingId === task.id}
                  onToggle={() => void toggle(task)}
                  onRun={() => void runNow(task)}
                  onDelete={() => void remove(task)}
                  onEditToggle={() =>
                    setEditingId((current) => (current === task.id ? null : task.id))}
                  onSave={(changes) => void saveEdit(task, changes)}
                />
              ))}
            </div>
          )}
        </div>
      </div>
    </aside>
  );
}

function EmptyScheduleState() {
  return (
    <div class="mt-8 rounded-lg border border-dashed border-line-strong bg-tint px-5 py-7 text-center">
      <div class="mx-auto mb-3 h-10 w-10 rounded-lg border border-line bg-tint grid place-items-center">
        <CalendarClock class="w-5 h-5 text-accent-blue" />
      </div>
      <h3 class="text-[14px] font-medium text-ink-100">No scheduled tasks</h3>
      <p class="mx-auto mt-1.5 max-w-sm text-[12.5px] leading-5 text-ink-400">
        Ask the agent to schedule work in this chat. It can create a one-time reminder or a recurring cron task.
      </p>
      <div class="mx-auto mt-4 max-w-sm rounded-md border border-line bg-inset px-3 py-2 text-left text-[12px] leading-5 text-ink-300">
        “Watch the deploy every 5 minutes and stop when it is healthy.”
      </div>
    </div>
  );
}

function ScheduledTaskCard({
  task,
  busyAction,
  actionsDisabled,
  editing,
  onToggle,
  onRun,
  onDelete,
  onEditToggle,
  onSave,
}: {
  task: ScheduledTask;
  busyAction: TaskAction | null;
  actionsDisabled: boolean;
  editing: boolean;
  onToggle: () => void;
  onRun: () => void;
  onDelete: () => void;
  onEditToggle: () => void;
  onSave: (changes: UpdateScheduledTaskInput) => void;
}) {
  const resumeAllowed = canResumeScheduledTask(task);
  const toggleDisabled = actionsDisabled || (!task.enabled && !resumeAllowed);
  const awaitingArm = isAwaitingArm(task);
  const toggleLabel = toggleActionLabel(task);

  return (
    <article class={`rounded-lg border px-3 py-3 ${task.enabled ? "border-line bg-tint" : "border-line bg-tint"}`}>
      <div class="flex items-start gap-2">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2 min-w-0">
            <h3 class="truncate text-[14px] font-medium text-ink-50">{task.name}</h3>
            <TaskStatus task={task} />
          </div>
          <div class="mt-1 truncate font-mono text-[11.5px] text-accent-blue/90" title={scheduleDefinition(task)}>
            {scheduleDefinition(task)}
          </div>
        </div>
        <span class="flex-none text-[11px] text-ink-400">{scheduleRunCount(task)}</span>
      </div>

      <p class="mt-2 line-clamp-3 whitespace-pre-wrap break-words text-[12.5px] leading-[1.55] text-ink-300" title={task.prompt}>
        {task.prompt}
      </p>

      <dl class="mt-3 grid grid-cols-2 gap-x-3 gap-y-2 rounded-md border border-line bg-inset px-2.5 py-2">
        <TaskDetail
          label="Next"
          value={task.enabled
            ? formatTimestamp(task.nextRunAt)
            : humanize(task.status || "paused")}
        />
        <TaskDetail label="Last run" value={formatTimestamp(task.lastRunAt)} />
        <TaskDetail label="Last result" value={task.lastRunStatus ? humanize(task.lastRunStatus) : "None"} tone={lastRunTone(task.lastRunStatus)} />
        <TaskDetail label="Owner" value={task.ownerEmail || "Unknown"} />
      </dl>

      {task.lastError && (
        <div class="mt-2 rounded-md border border-accent-red/20 bg-accent-red/[0.06] px-2.5 py-2 text-[11.5px] leading-4 text-accent-red break-words">
          {task.lastError}
        </div>
      )}

      {awaitingArm && (
        <div class="mt-2 rounded-md border border-accent-yellow/25 bg-accent-yellow/[0.08] px-2.5 py-2 text-[11.5px] leading-4 text-accent-yellow">
          Created by the agent and parked. Press Arm to put it on the clock.
        </div>
      )}

      <div class="mt-3 flex items-center gap-2">
        <button
          type="button"
          onClick={onToggle}
          disabled={toggleDisabled}
          class={`h-8 inline-flex items-center gap-1.5 rounded-md border px-2.5 text-[12px] disabled:opacity-45
                  ${awaitingArm
                    ? "border-accent-green/30 bg-accent-green/[0.08] text-accent-green hover:bg-accent-green/[0.14]"
                    : "border-line bg-tint text-ink-200 hover:bg-tint-strong"}`}
          title={
            task.enabled
              ? "Pause schedule"
              : resumeAllowed
                ? awaitingArm ? "Arm this schedule" : "Resume schedule"
                : `${humanize(task.status || "terminal")} tasks cannot be resumed`
          }
        >
          {busyAction === "toggle"
            ? <Loader class="w-3.5 h-3.5 animate-spin" />
            : task.enabled
              ? <Pause class="w-3.5 h-3.5" />
              : <Play class="w-3.5 h-3.5" />}
          {toggleLabel}
        </button>
        <button
          type="button"
          onClick={onRun}
          disabled={actionsDisabled}
          class="h-8 inline-flex items-center gap-1.5 rounded-md border border-line bg-tint px-2.5 text-[12px] text-ink-200 hover:bg-tint-strong disabled:opacity-45"
          title="Run this task now"
        >
          {busyAction === "run" ? <Loader class="w-3.5 h-3.5 animate-spin" /> : <Play class="w-3.5 h-3.5" />}
          Run now
        </button>
        <button
          type="button"
          onClick={onEditToggle}
          disabled={actionsDisabled}
          aria-pressed={editing}
          class={`h-8 w-8 rounded-md border grid place-items-center disabled:opacity-45
                  ${editing
                    ? "border-accent-blue/35 bg-accent-blue/[0.14] text-accent-blue"
                    : "border-line bg-tint text-ink-400 hover:bg-tint-strong hover:text-ink-100"}`}
          title={editing ? "Close editor" : "Edit scheduled task"}
          aria-label={`Edit ${task.name}`}
        >
          <Edit class="w-3.5 h-3.5" />
        </button>
        <button
          type="button"
          onClick={onDelete}
          disabled={actionsDisabled}
          class="ml-auto h-8 w-8 rounded-md border border-line bg-tint text-ink-400 grid place-items-center hover:bg-accent-red/[0.08] hover:text-accent-red disabled:opacity-45"
          title="Delete scheduled task"
          aria-label={`Delete ${task.name}`}
        >
          {busyAction === "delete" ? <Loader class="w-3.5 h-3.5 animate-spin" /> : <Trash class="w-3.5 h-3.5" />}
        </button>
      </div>

      {editing && (
        <EditTaskForm
          task={task}
          saving={busyAction === "save"}
          onCancel={onEditToggle}
          onSave={onSave}
        />
      )}
    </article>
  );
}

// Inline definition editor. Only fields the user changed are sent, so an
// untouched schedule field never resets scheduler state.
function EditTaskForm({
  task,
  saving,
  onCancel,
  onSave,
}: {
  task: ScheduledTask;
  saving: boolean;
  onCancel: () => void;
  onSave: (changes: UpdateScheduledTaskInput) => void;
}) {
  const [name, setName] = useState(task.name);
  const [prompt, setPrompt] = useState(task.prompt);
  const [cron, setCron] = useState(task.cron ?? "");
  const [at, setAt] = useState(task.at ? toLocalDateTimeInput(task.at) : "");
  const [timezone, setTimezone] = useState(task.timezone);
  const [maxRuns, setMaxRuns] = useState(task.maxRuns ? String(task.maxRuns) : "");
  const [formError, setFormError] = useState<string | null>(null);

  function submit(event: Event) {
    event.preventDefault();
    const changes: UpdateScheduledTaskInput = {};
    if (name.trim() !== task.name) changes.name = name.trim();
    if (prompt.trim() !== task.prompt) changes.prompt = prompt.trim();
    if (timezone.trim() !== task.timezone) changes.timezone = timezone.trim();
    if (task.kind === "cron" && cron.trim() !== (task.cron ?? "")) {
      changes.cron = cron.trim();
    }
    if (task.kind === "once" && at) {
      const millis = new Date(at).getTime();
      if (Number.isNaN(millis)) {
        setFormError("Enter a valid date and time.");
        return;
      }
      if (millis !== task.at) changes.at = millis;
    }
    const parsedMaxRuns = maxRuns.trim() === "" ? 0 : Number(maxRuns);
    if (!Number.isInteger(parsedMaxRuns) || parsedMaxRuns < 0) {
      setFormError("Max runs must be a non-negative whole number.");
      return;
    }
    if (parsedMaxRuns !== (task.maxRuns ?? 0)) changes.maxRuns = parsedMaxRuns;

    if (Object.keys(changes).length === 0) {
      onCancel();
      return;
    }
    setFormError(null);
    onSave(changes);
  }

  return (
    <form onSubmit={submit} class="mt-3 space-y-2 rounded-md border border-line bg-inset p-2.5">
      <EditField label="Name">
        <input
          type="text"
          value={name}
          onInput={(event) => setName((event.currentTarget as HTMLInputElement).value)}
          class="h-8 w-full rounded-md border border-line bg-inset px-2 text-[12.5px] text-ink-100 focus:outline-none focus:border-accent-blue/60"
        />
      </EditField>
      <EditField label="Prompt">
        <textarea
          value={prompt}
          onInput={(event) => setPrompt((event.currentTarget as HTMLTextAreaElement).value)}
          rows={4}
          class="w-full rounded-md border border-line bg-inset px-2 py-1.5 text-[12.5px] leading-5 text-ink-100 focus:outline-none focus:border-accent-blue/60"
        />
      </EditField>
      {task.kind === "cron" ? (
        <EditField label="Cron (five fields)">
          <input
            type="text"
            value={cron}
            onInput={(event) => setCron((event.currentTarget as HTMLInputElement).value)}
            placeholder="*/10 * * * *"
            class="h-8 w-full rounded-md border border-line bg-inset px-2 font-mono text-[12px] text-ink-100 focus:outline-none focus:border-accent-blue/60"
          />
        </EditField>
      ) : (
        <EditField label="Run at">
          <input
            type="datetime-local"
            value={at}
            onInput={(event) => setAt((event.currentTarget as HTMLInputElement).value)}
            class="h-8 w-full rounded-md border border-line bg-inset px-2 text-[12px] text-ink-100 focus:outline-none focus:border-accent-blue/60"
          />
        </EditField>
      )}
      <div class="grid grid-cols-2 gap-2">
        <EditField label="Timezone">
          <input
            type="text"
            value={timezone}
            onInput={(event) => setTimezone((event.currentTarget as HTMLInputElement).value)}
            placeholder="UTC"
            class="h-8 w-full rounded-md border border-line bg-inset px-2 text-[12px] text-ink-100 focus:outline-none focus:border-accent-blue/60"
          />
        </EditField>
        <EditField label="Max runs (0 = unlimited)">
          <input
            type="text"
            inputMode="numeric"
            value={maxRuns}
            onInput={(event) => setMaxRuns((event.currentTarget as HTMLInputElement).value)}
            placeholder="0"
            class="h-8 w-full rounded-md border border-line bg-inset px-2 text-[12px] text-ink-100 focus:outline-none focus:border-accent-blue/60"
          />
        </EditField>
      </div>
      {formError && (
        <div class="text-[11.5px] text-accent-red">{formError}</div>
      )}
      <div class="flex items-center justify-end gap-2 pt-0.5">
        <button
          type="button"
          onClick={onCancel}
          disabled={saving}
          class="h-8 rounded-md border border-line bg-tint px-2.5 text-[12px] text-ink-300 hover:bg-tint-strong disabled:opacity-45"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={saving}
          class="h-8 inline-flex items-center gap-1.5 rounded-md border border-accent-blue/35 bg-accent-blue/[0.14] px-3 text-[12px] font-medium text-accent-blue hover:bg-accent-blue/[0.2] disabled:opacity-45"
        >
          {saving && <Loader class="w-3.5 h-3.5 animate-spin" />}
          Save changes
        </button>
      </div>
    </form>
  );
}

function EditField({ label, children }: { label: string; children: ComponentChildren }) {
  return (
    <label class="block">
      <span class="mb-1 block text-[10px] uppercase tracking-wide text-ink-500">{label}</span>
      {children}
    </label>
  );
}

// datetime-local inputs want local wall-clock time without a zone suffix.
function toLocalDateTimeInput(timestamp: number): string {
  const date = new Date(timestamp);
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function TaskStatus({ task }: { task: ScheduledTask }) {
  const label = !task.enabled && task.status === "scheduled" ? "paused" : task.status || (task.enabled ? "scheduled" : "paused");
  const classes = task.enabled
    ? "border-accent-green/25 bg-accent-green/[0.08] text-accent-green"
    : "border-line bg-tint text-ink-400";
  return (
    <span class={`flex-none rounded-full border px-1.5 py-0.5 text-[9.5px] font-medium uppercase tracking-wide ${classes}`}>
      {humanize(label)}
    </span>
  );
}

function TaskDetail({
  label,
  value,
  tone = "text-ink-200",
}: {
  label: string;
  value: string;
  tone?: string;
}) {
  return (
    <div class="min-w-0">
      <dt class="text-[10px] uppercase tracking-wide text-ink-500">{label}</dt>
      <dd class={`mt-0.5 truncate text-[11.5px] ${tone}`} title={value}>{value}</dd>
    </div>
  );
}

function lastRunTone(status?: string): string {
  const normalized = (status || "").toLowerCase();
  if (["failed", "error"].includes(normalized)) return "text-accent-red";
  if (["success", "succeeded", "complete", "completed"].includes(normalized)) return "text-accent-green";
  return "text-ink-200";
}

function humanize(value: string): string {
  return value.replaceAll("_", " ");
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

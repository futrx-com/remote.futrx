import { ICON } from "./config.js";

export function actionIcon(paths, className = "scheduled-tasks-action-icon") {
  const icon = document.createElement("span");
  icon.className = className;
  icon.setAttribute("aria-hidden", "true");
  icon.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" focusable="false">${paths}</svg>`;
  return icon;
}

export function taskState(task, now = Date.now()) {
  if (task.activeRunId) return "Run pending";
  if (task.enabled) return "Scheduled";
  if ((task.kind === "once" && task.runCount > 0) || (task.maxRuns > 0 && task.runCount >= task.maxRuns)) return "Finished";
  if (task.kind === "once" && Date.parse(task.at) <= now) return "Time has passed";
  return "Paused";
}

export function isArchived(task) {
  return Boolean(task.archived || taskState(task) === "Finished");
}

export function readableTime(value, timezone = "UTC") {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return "Time unavailable";
  try {
    return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short", timeZone: timezone }).format(date) + ` (${timezone})`;
  } catch { return date.toISOString() + " (UTC)"; }
}

export function scheduleSummary(task) {
  if (task.kind === "once") return `Once, on ${readableTime(task.at, task.timezone)}`;
  const fields = (task.cron || "").trim().split(/\s+/);
  const [minute, hour, day, month, weekday] = fields;
  const zone = task.timezone || "UTC";
  if (fields.length === 5 && day === "*" && month === "*" && weekday === "*") {
    if (minute === "*" && hour === "*") return `Every minute (${zone})`;
    if (/^\*\/[1-9]\d*$/.test(minute) && hour === "*") return `Every ${minute.slice(2)} minutes (${zone})`;
    if (/^\d+$/.test(minute) && /^\d+$/.test(hour)) return `Every day at ${hour.padStart(2, "0")}:${minute.padStart(2, "0")} (${zone})`;
  }
  return task.nextRunAt ? `Repeats; next run on ${readableTime(task.nextRunAt, task.timezone)}` : "Recurring schedule; no next run while paused";
}

export function friendlyError(error) {
  const message = String(error?.message || error || "Unknown error");
  if (/future RFC3339|choose at or cron/.test(message)) return "This reminder’s time has passed. Use Run now, or ask the agent to create a reminder for a future time.";
  if (/maxRuns/.test(message)) return "This task has finished its planned runs. Ask the agent to create a new schedule.";
  if (/pending run|claim changed/.test(message)) return "This task already has a run pending. Refresh to see its latest status.";
  if (/task not found|404/.test(message)) return "This task is no longer available. Refresh the list.";
  if (/fetch|network|timeout|backend|running/i.test(message)) return "Couldn’t reach Scheduled Tasks. Check your connection and that the application is running, then try again.";
  return `Couldn’t complete the request. ${message}`;
}

export function renderTask(task, action) {
  const row = document.createElement("section");
  const title = document.createElement("h3"); title.textContent = task.name;
  const info = document.createElement("p");
  info.className = "scheduled-tasks-state";
  const state = taskState(task);
  info.textContent = isArchived(task) ? `Archived · ${state}` : state;
  if (task.activeRunId) info.textContent += " — waiting for the agent or running; updates automatically";
  const prompt = document.createElement("p"); prompt.textContent = task.prompt;
  const what = document.createElement("span");
  what.className = "scheduled-tasks-label";
  what.textContent = "What the agent will do";
  const when = document.createElement("p");
  when.className = "scheduled-tasks-when";
  when.textContent = scheduleSummary(task);
  const feedback = document.createElement("p");
  feedback.className = "scheduled-tasks-error";
  feedback.setAttribute("role", "alert");
  const actions = document.createElement("div");
  actions.className = "scheduled-tasks-actions";
  if (!isArchived(task) && (task.enabled || state === "Paused")) actions.append(action(task.enabled ? "Pause" : "Resume", task, "PATCH", "", { enabled: !task.enabled }, feedback));
  if (!isArchived(task) && !task.activeRunId) actions.append(action("Run now", task, "POST", "/run", undefined, feedback));
  if (!isArchived(task)) actions.append(action("Archive", task, "PATCH", "", { archived: true }, feedback));
  else if (state !== "Finished") actions.append(action("Restore", task, "PATCH", "", { archived: false }, feedback));
  actions.append(action("Delete", task, "DELETE", "", undefined, feedback));
  row.append(title, info, what, prompt, when);
  if (state === "Time has passed" || state === "Finished") {
    const note = document.createElement("p");
    note.className = "scheduled-tasks-note";
    note.textContent = isArchived(task) ? "No more automatic runs. Ask the agent to schedule it again." : "No more automatic runs. Run it now, or ask the agent to schedule it again.";
    row.append(note);
  }
  if (task.lastError && !task.activeRunId) feedback.textContent = `Last run: ${friendlyError(task.lastError)}`;
  row.append(feedback, actions);
  return row;
}

export function renderEmpty() {
  const empty = document.createElement("section");
  empty.className = "scheduled-tasks-empty";
  const symbol = document.createElement("span");
  symbol.className = "scheduled-tasks-badge";
  symbol.innerHTML = ICON;
  symbol.setAttribute("aria-hidden", "true");
  const title = document.createElement("h3");
  title.textContent = "No scheduled tasks";
  const help = document.createElement("p");
  help.textContent = "Ask the agent to schedule work in this chat. It can create a one-time reminder or a recurring cron task.";
  const example = document.createElement("blockquote");
  example.textContent = "“Watch the deploy every 5 minutes and stop when it is healthy.”";
  empty.append(symbol, title, help, example);
  return empty;
}

export function renderSkeleton() {
  const skeleton = document.createElement("section");
  skeleton.className = "scheduled-tasks-skeleton";
  skeleton.setAttribute("aria-hidden", "true");
  skeleton.innerHTML = '<span class="scheduled-tasks-skeleton-line scheduled-tasks-skeleton-title"></span><span class="scheduled-tasks-skeleton-line scheduled-tasks-skeleton-meta"></span><span class="scheduled-tasks-skeleton-line"></span><span class="scheduled-tasks-skeleton-line scheduled-tasks-skeleton-meta"></span><span class="scheduled-tasks-skeleton-actions"><i></i><i></i><i></i></span>';
  return skeleton;
}

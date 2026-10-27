import { ACTION_ICONS } from "./config.js";
import { actionIcon, friendlyError, isArchived, renderTask, renderEmpty, renderSkeleton } from "./taskView.js";
import { showDeleteConfirmation } from "./deleteConfirmation.js";
import { openPopover } from "./popover.js";

export function openTasks(remote, context, anchor, onClose = () => {}) {
  const openPopup = anchor
    ? (options) => openPopover(anchor, onClose, options)
    : remote.ui.openPopup;
  return openPopup({
    title: "Scheduled tasks", width: 360,
    mount: (body, chrome) => {
      body.classList.add("scheduled-tasks-panel");
      let closed = false;
      let loading = false;
      let showArchived = false;
      let renderedSnapshot;
      let pendingDelete = false;
      const target = { projectId: context.projectId };
      const status = document.createElement("p");
      status.setAttribute("role", "status");
      const list = document.createElement("div");
      const archiveToggle = document.createElement("button");
      archiveToggle.type = "button";
      archiveToggle.className = "scheduled-tasks-action scheduled-tasks-archive-toggle";
      const archiveLabel = document.createElement("span");
      archiveLabel.className = "scheduled-tasks-archive-label";
      archiveLabel.textContent = "Show archived tasks";
      archiveToggle.append(
        actionIcon(ACTION_ICONS.Archive), archiveLabel,
        actionIcon('<path d="m6 9 6 6 6-6"/>', "scheduled-tasks-action-icon scheduled-tasks-archive-chevron"),
      );
      archiveToggle.setAttribute("aria-expanded", "false");
      const refresh = document.createElement("button");
      refresh.type = "button"; refresh.textContent = "Refresh";
      refresh.setAttribute("aria-label", "Refresh scheduled tasks");
      if (chrome) {
        refresh.textContent = "↻";
        chrome.header.insertBefore(refresh, chrome.dismiss);
        body.replaceChildren(status, list, archiveToggle);
      } else body.replaceChildren(status, refresh, list, archiveToggle);
      const action = (label, task, method, suffix = "", data, feedback = status) => {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "scheduled-tasks-action";
        if (label === "Run now") button.classList.add("scheduled-tasks-action-primary");
        if (method === "DELETE") button.classList.add("scheduled-tasks-action-destructive");
        button.title = `${label}: ${task.name}`;
        const text = document.createElement("span");
        text.textContent = label;
        button.append(actionIcon(ACTION_ICONS[label]), text);
        const execute = async () => {
          if (closed || button.disabled) return;
          button.disabled = true;
          feedback.textContent = "";
          try {
            await remote.backend.call(`tasks/${encodeURIComponent(task.id)}${suffix}`, {
              ...target, method, ...(data ? { body: data } : {}),
            });
            if (method === "DELETE") pendingDelete = false;
            await load();
          } catch (error) { if (!closed) feedback.textContent = friendlyError(error); }
          finally { if (!closed) button.disabled = false; }
        };
        button.addEventListener("click", () => {
          if (method !== "DELETE") return execute();
          if (pendingDelete || closed) return;
          pendingDelete = true;
          showDeleteConfirmation(button, task.name, {
            onConfirm: execute,
            onDismiss: () => { pendingDelete = false; },
            isClosed: () => closed,
          });
        });
        return button;
      };
      async function load(quiet = false) {
        if (closed || loading || pendingDelete) return;
        loading = true; refresh.disabled = true; status.textContent = "";
        body.setAttribute("aria-busy", "true");
        if (!quiet) {
          renderedSnapshot = undefined;
          list.replaceChildren();
          if (chrome) {
            chrome.count.textContent = "";
            chrome.count.classList.add("scheduled-tasks-skeleton-count");
          }
          for (let index = 0; index < 3; index++) {
            list.append(renderSkeleton());
          }
        }
        try {
          const allTasks = await remote.backend.call("tasks", {
            ...target,
            query: { chatId: context.chatId },
          });
          if (closed) return;
          allTasks.sort((a, b) => a.id.localeCompare(b.id));
          const snapshot = JSON.stringify([showArchived, allTasks]);
          if (quiet && snapshot === renderedSnapshot) return;
          renderedSnapshot = snapshot;
          const archived = allTasks.filter(isArchived);
          const tasks = allTasks.filter((task) => showArchived || !isArchived(task));
          archiveLabel.textContent = `${showArchived ? "Hide" : "Show"} archived tasks${archived.length ? ` (${archived.length})` : ""}`;
          list.replaceChildren();
          if (chrome) chrome.count.textContent = tasks.length ? `${tasks.length} ${tasks.length === 1 ? "task" : "tasks"}` : "No tasks";
          tasks.sort((a, b) => Number(isArchived(a)) - Number(isArchived(b)) || (a.nextRunAt || Infinity) - (b.nextRunAt || Infinity));
          for (const task of tasks) {
            list.append(renderTask(task, action));
          }
          status.textContent = "";
          if (!tasks.length) {
            list.append(renderEmpty());
          }
        } catch (error) {
          if (!closed) {
            if (!quiet) list.replaceChildren();
            status.className = "scheduled-tasks-error";
            status.textContent = friendlyError(error) + " Use Refresh to retry.";
            if (chrome) chrome.count.textContent = "Unable to load tasks";
          }
        }
        finally {
          loading = false;
          if (!closed) {
            refresh.disabled = false;
            body.setAttribute("aria-busy", "false");
            chrome?.count.classList.remove("scheduled-tasks-skeleton-count");
          }
        }
      }
      const refreshTasks = () => { void load(); };
      const toggleArchived = () => {
        showArchived = !showArchived;
        archiveToggle.setAttribute("aria-expanded", String(showArchived));
        void load(true);
      };
      archiveToggle.addEventListener("click", toggleArchived);
      refresh.addEventListener("click", refreshTasks);
      const timer = setInterval(() => { void load(true); }, 3000);
      void load();
      return () => { closed = true; clearInterval(timer); archiveToggle.removeEventListener("click", toggleArchived); refresh.removeEventListener("click", refreshTasks); body.classList.remove("scheduled-tasks-panel"); };
    },
  });
}

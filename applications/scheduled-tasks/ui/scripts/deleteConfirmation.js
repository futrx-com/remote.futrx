import { ACTION_ICONS } from "./config.js";
import { actionIcon } from "./taskView.js";

// Owns temporary confirmation UI; requests and panel polling remain with the caller.
export function showDeleteConfirmation(trigger, name, { onConfirm, onDismiss, isClosed }) {
  const actions = trigger.parentElement;
  const originalButtons = [...actions.children];
  originalButtons.forEach((item) => { item.hidden = true; });
  const confirmation = document.createElement("div");
  confirmation.className = "scheduled-tasks-delete-confirmation";
  confirmation.setAttribute("role", "group");
  confirmation.setAttribute("aria-label", `Delete ${name}?`);
  const heading = document.createElement("div");
  heading.className = "scheduled-tasks-delete-heading";
  const title = document.createElement("strong");
  title.textContent = "Delete this task?";
  heading.append(actionIcon(ACTION_ICONS.Delete, "scheduled-tasks-delete-icon"), title);
  const taskName = document.createElement("p");
  taskName.className = "scheduled-tasks-delete-name";
  taskName.textContent = name;
  const description = document.createElement("p");
  description.textContent = "Its schedule will be permanently removed. This cannot be undone.";
  const controls = document.createElement("div");
  controls.className = "scheduled-tasks-delete-controls";
  const cancel = document.createElement("button");
  cancel.type = "button";
  cancel.textContent = "Cancel";
  const confirm = document.createElement("button");
  confirm.type = "button";
  confirm.className = "scheduled-tasks-delete-submit";
  confirm.textContent = "Delete task";
  const dismiss = () => {
    confirmation.remove();
    originalButtons.forEach((item) => { item.hidden = false; });
    onDismiss();
    if (!isClosed()) trigger.focus();
  };
  cancel.addEventListener("click", dismiss);
  confirmation.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      if (!confirm.disabled) dismiss();
    }
  });
  confirm.addEventListener("click", async () => {
    if (confirm.disabled || isClosed()) return;
    confirm.disabled = true;
    cancel.disabled = true;
    confirm.textContent = "Deleting…";
    await onConfirm();
    dismiss();
  });
  controls.append(cancel, confirm);
  confirmation.append(heading, taskName, description, controls);
  actions.append(confirmation);
  cancel.focus();
}

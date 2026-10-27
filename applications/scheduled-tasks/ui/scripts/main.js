import { ICON } from "./config.js";
import { openTasks } from "./tasksPanel.js";

export function installed(remote, projectId) {
  return Boolean(projectId && remote.backend.instances.some(
    (instance) => instance.scope === "project" && instance.projectId === projectId,
  ));
}

export default function activate(remote) {
  remote.ui.register(remote.slots.chatHeaderActions, (host, context) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "scheduled-tasks-trigger";
    button.innerHTML = ICON;
    button.setAttribute("aria-label", "Scheduled tasks");
    button.setAttribute("aria-haspopup", "dialog");
    button.setAttribute("aria-expanded", "false");
    button.title = "View scheduled tasks";
    let popup;
    const toggle = () => {
      if (popup) { popup.close(); return; }
      popup = openTasks(remote, context, button, () => {
        popup = undefined;
        button.setAttribute("aria-expanded", "false");
      });
      button.setAttribute("aria-expanded", "true");
    };
    button.addEventListener("click", toggle);
    host.append(button);
    return () => { popup?.close(); button.removeEventListener("click", toggle); };
  }, {
    when: (context) => Boolean(context.chatId && installed(remote, context.projectId)),
  });
}

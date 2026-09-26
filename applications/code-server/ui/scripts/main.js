import { openSettingsForm } from "./settingsForm.js";

const ICON = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" ' +
  'stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">' +
  '<path d="m16 18 6-6-6-6M8 6l-6 6 6 6"/></svg>';

// Chat cwd is the host path to the bind-mounted project workspace.
const WORKSPACE = /^\/var\/lib\/remote\/projects\/([a-z0-9][a-z0-9-]*)\/workspace(?:\/(.*))?$/;

export function workspaceIdeUrl(cwd, origin = window.location.origin) {
  const match = WORKSPACE.exec(cwd || "");
  if (!match) return null;
  const base = new URL(origin);
  base.pathname = `/${match[1]}/code/`;
  base.searchParams.set("folder", match[2] ? `/workspace/${match[2]}` : "/workspace");
  return base.toString();
}

export default function activate(remote) {
  remote.ui.addIconButton(remote.slots.chatHeaderActions, {
    icon: ICON,
    label: "Workspace IDE",
    title: "Open workspace in IDE",
    when: (context) => Boolean(context.projectId && workspaceIdeUrl(context.cwd)),
    onClick: (context) => {
      const url = workspaceIdeUrl(context.cwd);
      if (url) window.open(url, "_blank", "noopener,noreferrer");
    },
  });

  remote.ui.addButton(remote.slots.applicationCardActions, {
    label: "Settings",
    title: "Edit Code Server settings",
    when: (context) => context.instance?.applicationId === remote.application.id &&
      context.instance?.status === "running",
    onClick: (context) => openSettingsForm(remote, context.instance.id),
  });
}

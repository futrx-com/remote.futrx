import { fileIdeUrl, workspaceIdeUrl } from "./editorUrls.js";
import { openSettingsForm } from "./settingsForm.js";

const ICON = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" ' +
  'stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">' +
  '<path d="m16 18 6-6-6-6M8 6l-6 6 6 6"/></svg>';

export default function activate(remote) {
  // Read current availability; the project slug keeps the address stable on reinstall.
  const isInstalled = (projectId) => remote.backend.instances.some(
    (instance) => instance.scope === "project" && instance.projectId === projectId,
  );
  remote.files.registerOpener((request) =>
    fileIdeUrl(request, isInstalled(request.projectId), remote.application.web?.subdomain),
  );
  remote.ui.addIconButton(remote.slots.chatHeaderActions, {
    icon: ICON,
    label: "Workspace IDE",
    title: "Open workspace in IDE",
    when: (context) => Boolean(
      context.projectId && workspaceIdeUrl(
        context.cwd, isInstalled(context.projectId), remote.application.web?.subdomain,
      ),
    ),
    onClick: (context) => {
      const url = workspaceIdeUrl(
        context.cwd, isInstalled(context.projectId), remote.application.web?.subdomain,
      );
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

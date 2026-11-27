import { createShellConnection } from "./connection.js";
import { createShellApi } from "./shellApi.js";
import { createTerminalView } from "./terminalView.js";
import { workspaceLocation } from "./workspacePath.js";
import { ICON } from "./config/drawer.js";
import { mountTerminal } from "./controllers/terminalDrawerController.js";

export default function activate(remote) {
  // Remote builds without drawers predate this application's only surface.
  if (!remote.ui.registerDrawer) return;
  // The shell belongs to the chat and runs in this project's installation.
  const reachable = (target) => Boolean(
    target.projectId && target.chatId && workspaceLocation(target.cwd) &&
      remote.backend?.instances?.some((instance) => instance.projectId === target.projectId),
  );

  remote.ui.registerDrawer({
    id: "shell",
    title: "Terminal",
    label: "Container terminal",
    icon: ICON,
    when: reachable,
    mount: (body, context) => mountTerminal(body, context, {
      reachable: reachable(context),
      createApi: () => createShellApi(remote, context.projectId),
      createView: createTerminalView,
      createConnection: createShellConnection,
    }),
  });
}

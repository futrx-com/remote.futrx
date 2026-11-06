import { createShellConnection } from "./connection.js";
import { createShellApi } from "./shellApi.js";
import { createTerminalView } from "./terminalView.js";
import { workspaceLocation } from "./workspacePath.js";

const ICON = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" ' +
  'stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
  '<path d="m4 17 6-6-6-6M12 19h8"/></svg>';

// The connection's state, as the header words it.
const STATUS_LABELS = {
  connecting: "Connecting",
  connected: "Connected",
  reconnecting: "Reconnecting",
  ended: "Closed",
};

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

export function mountTerminal(body, context, { reachable, createApi, createView, createConnection }) {
  if (!reachable) {
    context.setStatus({ label: "Unavailable for this chat", active: false });
    return;
  }
  const path = workspaceLocation(context.cwd).path;
  const show = (status) => context.setStatus({
    label: `${STATUS_LABELS[status]} - ${path}`,
    active: status === "connected",
  });

  let connection = null;
  const view = createView(body, {
    onData: (data) => connection?.input(data),
    onResize: (size) => connection?.resize(size),
    onVisible: (shown) => connection?.setVisible(shown),
  });
  connection = createConnection({
    api: createApi(),
    session: context.chatId,
    cwd: path,
    size: view.size,
    onOutput: view.write,
    // Every attach starts from the shell's recent output, which would
    // otherwise appear twice after a reconnect.
    onReset: view.reset,
    onEnded: () => view.notice("Shell exited. Press Enter to start a new one."),
    onConnected: view.fitAndResize,
    onStatus: show,
  });
  const resume = () => connection.resume();
  window.addEventListener("online", resume);
  connection.start();

  return () => {
    window.removeEventListener("online", resume);
    connection.dispose();
    view.dispose();
  };
}

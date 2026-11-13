import { STATUS_LABELS } from "../config/drawer.js";
import { workspaceLocation } from "../workspacePath.js";

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

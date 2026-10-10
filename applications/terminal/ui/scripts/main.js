import { terminalUrl, workspaceLocation } from "./workspacePath.js";

const ICON = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" ' +
  'stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
  '<path d="m4 17 6-6-6-6M12 19h8"/></svg>';

// What the terminal page reports about its connection, as the header words it.
const STATUS_LABELS = {
  connecting: "Connecting",
  connected: "Connected",
  reconnecting: "Reconnecting",
  ended: "Closed",
};

export default function activate(remote) {
  // Remote builds without drawers predate this application's only surface.
  if (!remote.ui.registerDrawer) return;
  const subdomain = () => remote.application.web?.subdomain;

  remote.ui.registerDrawer({
    id: "shell",
    title: "Terminal",
    label: "Container terminal",
    icon: ICON,
    when: (target) => Boolean(
      target.projectId && terminalUrl(target.cwd, target.chatId, subdomain()),
    ),
    mount: (body, context) => mountTerminal(body, context, subdomain()),
  });
}

function mountTerminal(body, context, subdomain) {
  const url = terminalUrl(context.cwd, context.chatId, subdomain);
  if (!url) {
    context.setStatus({ label: "Unavailable for this chat", active: false });
    return;
  }
  const path = workspaceLocation(context.cwd).path;
  const origin = new URL(url).origin;
  const show = (status) => context.setStatus({
    label: `${STATUS_LABELS[status]} - ${path}`,
    active: status === "connected",
  });

  const frame = document.createElement("iframe");
  frame.title = "Terminal";
  frame.style.cssText = "display:block;width:100%;height:100%;border:0;background:transparent";
  frame.src = url;

  // The page runs on the project's application origin and reports its
  // connection state by message; anything from elsewhere is ignored.
  const onMessage = (event) => {
    if (event.origin !== origin || event.source !== frame.contentWindow) return;
    const status = event.data?.source === "remote-terminal" ? event.data.status : null;
    if (Object.hasOwn(STATUS_LABELS, status)) show(status);
  };
  window.addEventListener("message", onMessage);
  show("connecting");
  body.appendChild(frame);

  return () => {
    window.removeEventListener("message", onMessage);
    frame.remove();
  };
}

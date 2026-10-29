import { REFRESH_ICON } from "../config/drawer.js";

export function mountFilesDrawer(body, context, { url, syncTheme, createFileOpener }) {
  if (!url) {
    context.setStatus({
      label: "Unavailable for this chat",
      active: false,
    });
    return;
  }
  const origin = new URL(url).origin;
  const frame = document.createElement("iframe");
  frame.title = "Workspace files";
  frame.src = url;
  frame.style.cssText =
    "display:block;width:100%;height:100%;border:0;background:transparent";
  const send = (message) =>
    frame.contentWindow?.postMessage(
      { source: "remote-file-manager-host", ...message },
      origin,
    );
  const showState = ({ subtitle, loading }) => {
    context.setStatus({ label: subtitle });
    context.setActions?.([
      {
        id: "refresh",
        label: "Refresh files",
        icon: REFRESH_ICON,
        busy: loading,
        onClick: () => send({ type: "refresh" }),
      },
    ]);
  };
  showState({ subtitle: "Loading...", loading: true });
  const stopTheme = syncTheme(frame, send);
  const openFile = createFileOpener(context, (path, url) =>
    send({ type: "blocked", path, url }),
  );
  const onMessage = (event) => {
    if (
      event.source !== frame.contentWindow ||
      event.origin !== origin ||
      event.data?.source !== "remote-file-manager"
    )
      return;
    if (event.data.type === "state") {
      showState({
        subtitle: String(event.data.subtitle ?? ""),
        loading: Boolean(event.data.loading),
      });
      return;
    }
    if (event.data.type !== "open") return;
    const path = event.data.path;
    if (
      typeof path !== "string" ||
      !path ||
      path.startsWith("/") ||
      path.split("/").some((part) => part === ".." || part === ".")
    )
      return;
    openFile(path);
  };
  window.addEventListener("message", onMessage);
  body.append(frame);
  return () => {
    stopTheme();
    window.removeEventListener("message", onMessage);
    frame.remove();
  };
}

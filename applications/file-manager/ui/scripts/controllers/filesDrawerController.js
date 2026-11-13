import { REFRESH_ICON } from "../config/drawer.js";

// Paths come from the project container's listings, so a click is checked
// before it reaches an opener.
function workspaceRelative(path) {
  return (
    typeof path === "string" &&
    Boolean(path) &&
    !path.startsWith("/") &&
    !path.split("/").some((part) => part === ".." || part === ".")
  );
}

export function mountFilesDrawer(
  body,
  context,
  { api, createLayout, createBrowser, createView, createFileOpener, whenSettled },
) {
  if (!api) {
    context.setStatus({
      label: "Unavailable for this chat",
      active: false,
    });
    return;
  }
  const layout = createLayout(body);
  const { search, clear } = layout;
  const browser = createBrowser(api, render);
  const openPath = createFileOpener(context, (path, url) => {
    if (/^https?:\/\//i.test(url)) browser.showBlocked(path, url);
  });
  const view = createView(layout, {
    toggle: browser.toggle,
    openFile(node) {
      if (!workspaceRelative(node.path)) return;
      browser.clearBlocked();
      openPath(node.path);
    },
    dismissBlocked: browser.dismissBlocked,
    downloadUrl: api.downloadUrl,
    downloadFolder(node) {
      if (node.path && !workspaceRelative(node.path)) return;
      void browser.downloadFolder(node.path);
    },
  });

  // The header follows every change at once; the tree is drawn only once the
  // opening pane has stopped growing.
  let settled = false;
  function render() {
    if (settled) view.render(browser.state);
    context.setStatus({ label: browser.subtitle() });
    context.setActions?.([
      {
        id: "refresh",
        label: "Refresh files",
        icon: REFRESH_ICON,
        busy: browser.state.rootLoading,
        onClick: () => browser.reset(),
      },
    ]);
  }

  const onInput = () => browser.search(search.value);
  const onClear = () => {
    search.value = "";
    browser.search(search.value);
    search.focus();
  };
  search.addEventListener("input", onInput);
  clear.addEventListener("click", onClear);
  const stopWaiting = whenSettled(layout.root, () => {
    settled = true;
    render();
  });
  browser.reset();

  return () => {
    stopWaiting();
    browser.dispose();
    search.removeEventListener("input", onInput);
    clear.removeEventListener("click", onClear);
    layout.root.remove();
  };
}

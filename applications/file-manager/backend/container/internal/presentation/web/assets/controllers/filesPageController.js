export function mountFilesPage(
  { search, clear, entries },
  { createBrowser, createView, bridge, downloadUrl, download },
) {
  const browser = createBrowser(render);
  const view = createView(
    { search, clear, entries },
    {
      toggle: browser.toggle,
      openFile,
      dismissBlocked: browser.dismissBlocked,
      downloadUrl,
    },
  );

  function render() {
    view.render(browser.state);
    publishState();
  }

  function publishState() {
    bridge.post({
      type: "state",
      subtitle: browser.subtitle(),
      loading: browser.state.rootLoading,
    });
  }

  function openFile(node) {
    if (!bridge.embedded) {
      download(downloadUrl(node));
      return;
    }
    browser.clearBlocked();
    bridge.post({ type: "open", path: node.path });
  }

  search.addEventListener("input", () => browser.search(search.value));
  clear.addEventListener("click", () => {
    search.value = "";
    browser.search(search.value);
    search.focus();
  });

  window.addEventListener("message", (event) => {
    if (!bridge.accepts(event)) return;
    if (event.data.type === "refresh") {
      browser.reset();
      return;
    }
    if (
      event.data.type === "blocked" &&
      typeof event.data.path === "string" &&
      typeof event.data.url === "string" &&
      /^https?:\/\//i.test(event.data.url)
    ) {
      browser.showBlocked(event.data.path, event.data.url);
      return;
    }
    if (event.data.type !== "theme") return;
    view.applyTheme(event.data);
  });

  browser.reset();
}

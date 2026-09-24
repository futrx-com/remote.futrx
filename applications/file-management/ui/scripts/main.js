import { createInitialState, reduce } from "./browserState.js";
import { openAction } from "./fileKinds.js";
import { renderBrowse, renderSearch } from "./tree.js";

const FOLDER_ICON =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" ' +
  'stroke-linecap="round" stroke-linejoin="round"><path d="M3.5 7.5h6l2-2h9v13h-17z"/></svg>';

const SEARCH_DEBOUNCE_MS = 250;

export default function activate(remote) {
  remote.ui.addWorkspacePane({
    id: "files",
    label: "Files",
    title: "Browse workspace files",
    icon: FOLDER_ICON,
    width: 560,
    order: -100,
    when: ({ cwd }) => Boolean(cwd) && remote.backend.available,
    render: (host, context) => mountFileManager(host, remote, context),
  });
}

function mountFileManager(host, remote, context) {
  let state = createInitialState();
  let disposed = false;
  let loadToken = 0;
  let searchToken = 0;
  let searchTimer = null;
  const controllers = new Set();
  const target = { chatId: context.chatId, projectId: context.projectId };

  const shell = document.createElement("section");
  shell.className = "fm-shell";
  const tools = document.createElement("div");
  tools.className = "fm-tools";
  const subtitle = document.createElement("span");
  subtitle.className = "fm-subtitle";
  const refresh = document.createElement("button");
  refresh.type = "button";
  refresh.className = "fm-tool-button";
  refresh.title = "Refresh";
  refresh.setAttribute("aria-label", "Refresh files");
  refresh.textContent = "↻";
  tools.append(subtitle, refresh);

  const search = document.createElement("div");
  search.className = "fm-search";
  const searchMark = document.createElement("span");
  searchMark.className = "fm-search__mark";
  searchMark.setAttribute("aria-hidden", "true");
  searchMark.textContent = "⌕";
  const input = document.createElement("input");
  input.type = "search";
  input.className = "fm-search__input";
  input.placeholder = "Search all files…";
  input.autocomplete = "off";
  input.setAttribute("aria-label", "Search all files");
  const clear = document.createElement("button");
  clear.type = "button";
  clear.className = "fm-search__clear";
  clear.setAttribute("aria-label", "Clear search");
  clear.textContent = "×";
  search.append(searchMark, input, clear);

  const content = document.createElement("div");
  content.className = "fm-content";
  shell.append(tools, search, content);
  host.append(shell);

  const dispatch = (action) => {
    if (disposed) return;
    state = reduce(state, action);
    render();
  };

  const render = () => {
    if (disposed) return;
    const resultCount = state.searchResults?.length;
    subtitle.textContent = resultCount !== null && resultCount !== undefined
      ? `${resultCount} result${resultCount === 1 ? "" : "s"}${state.searchTruncated ? "+" : ""}`
      : state.rootLoading ? "Loading…" : "workspace";
    refresh.disabled = state.rootLoading;
    refresh.classList.toggle("fm-tool-button--busy", state.rootLoading);
    clear.hidden = !state.query && !state.searching;
    clear.textContent = state.searching ? "…" : "×";
    if (state.searchResults !== null) renderSearch(content, state, actions);
    else renderBrowse(content, state, actions);
  };

  const backendCall = (path, query = {}) => {
    const controller = new AbortController();
    controllers.add(controller);
    return remote.backend.call(path, { ...target, query, signal: controller.signal })
      .finally(() => controllers.delete(controller));
  };

  const loadDirectory = async (path, token) => {
    dispatch({ type: "directory-load-started", path });
    try {
      const listing = await backendCall("files", { path });
      if (disposed || token !== loadToken) return;
      dispatch({
        type: "directory-load-succeeded",
        path,
        entries: listing.entries ?? [],
        truncated: Boolean(listing.truncated),
      });
    } catch (error) {
      if (disposed || token !== loadToken || error?.name === "AbortError") return;
      dispatch({ type: "directory-load-failed", path, error: errorMessage(error) });
    }
  };

  const reset = () => {
    loadToken += 1;
    searchToken += 1;
    cancelSearch();
    for (const controller of controllers) controller.abort();
    controllers.clear();
    state = reduce(state, { type: "reset" });
    input.value = "";
    render();
    void loadDirectory("", loadToken);
  };

  const toggle = (path) => {
    const opening = !state.expanded.has(path);
    state = reduce(state, { type: "directory-toggled", path });
    render();
    if (opening && !state.childrenByDir.has(path) && !state.loading.has(path)) {
      void loadDirectory(path, loadToken);
    }
  };

  const backendUrl = (route, path) => {
    const url = new URL(remote.backend.url(route, target), window.location.origin);
    if (path) url.searchParams.set("path", path);
    return `${url.pathname}${url.search}${url.hash}`;
  };

  const downloadUrl = (node) => backendUrl(
    node.isDir ? "files/download-folder" : "files/download",
    node.path,
  );

  const open = (node) => {
    if (node.isDir) return;
    const action = openAction(node.name);
    if (action.action === "media") {
      remote.ui.openMedia({
        url: backendUrl("files/media", node.path),
        name: node.name,
        kind: action.kind,
      });
      return;
    }
    if (action.action === "ide") {
      const workspacePath = `/workspace/${node.path}`;
      const url = `/api/chats/${encodeURIComponent(context.chatId)}/ide-open?path=${encodeURIComponent(workspacePath)}`;
      window.open(url, "_blank", "noopener");
      return;
    }
    window.location.assign(downloadUrl(node));
  };

  const actions = { toggle, open, downloadUrl };

  const runSearch = (query, token) => {
    dispatch({ type: "search-started" });
    void backendCall("files/search", { q: query })
      .then((result) => {
        if (disposed || token !== searchToken) return;
        dispatch({
          type: "search-succeeded",
          entries: result.entries ?? [],
          truncated: Boolean(result.truncated),
        });
      })
      .catch((error) => {
        if (disposed || token !== searchToken || error?.name === "AbortError") return;
        dispatch({ type: "search-failed", error: errorMessage(error) });
      });
  };

  const cancelSearch = () => {
    if (searchTimer !== null) window.clearTimeout(searchTimer);
    searchTimer = null;
  };

  const onInput = () => {
    cancelSearch();
    const query = input.value;
    state = reduce(state, { type: "query-changed", query });
    searchToken += 1;
    const token = searchToken;
    if (Array.from(query.trim()).length < 2) {
      state = reduce(state, { type: "search-idle" });
      render();
      return;
    }
    state = reduce(state, { type: "search-started" });
    render();
    searchTimer = window.setTimeout(() => runSearch(query.trim(), token), SEARCH_DEBOUNCE_MS);
  };

  const onClear = () => {
    input.value = "";
    onInput();
    input.focus();
  };
  const onRefresh = () => reset();
  input.addEventListener("input", onInput);
  clear.addEventListener("click", onClear);
  refresh.addEventListener("click", onRefresh);

  render();
  reset();

  return () => {
    disposed = true;
    cancelSearch();
    for (const controller of controllers) controller.abort();
    controllers.clear();
    input.removeEventListener("input", onInput);
    clear.removeEventListener("click", onClear);
    refresh.removeEventListener("click", onRefresh);
  };
}

function errorMessage(error) {
  return error instanceof Error ? error.message : "File request failed";
}

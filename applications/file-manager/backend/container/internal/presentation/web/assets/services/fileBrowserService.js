import { SEARCH_DEBOUNCE_MS } from "../config/files.js";

export function createFileBrowser(api, changed) {
  const state = {
    query: "",
    rootLoading: false,
    rootEntries: [],
    rootError: "",
    expanded: new Set(),
    loadingDirs: new Set(),
    childrenByDir: new Map(),
    errorByDir: new Map(),
    truncatedDirs: new Set(),
    blocked: null,
    searchResults: null,
    searchTruncated: false,
    searching: false,
    searchError: "",
  };
  let revision = 0;
  let controller = new AbortController();
  let searchTimer;
  let searchController;

  function subtitle() {
    if (state.searchResults) {
      const count = state.searchResults.length;
      return `${count} result${count === 1 ? "" : "s"}${state.searchTruncated ? "+" : ""}`;
    }
    return state.rootLoading ? "Loading..." : "workspace";
  }

  async function loadDirectory(path, token) {
    state.loadingDirs.add(path);
    if (path === "") state.rootLoading = true;
    changed();
    try {
      const result = await api.list(path, controller.signal);
      if (token !== revision) return;
      const list = result.entries || [];
      if (path === "") state.rootEntries = list;
      state.childrenByDir.set(path, list);
      state.errorByDir.delete(path);
      if (path === "") state.rootError = "";
      if (result.truncated) state.truncatedDirs.add(path);
      else state.truncatedDirs.delete(path);
    } catch (error) {
      if (error.name === "AbortError" || token !== revision) return;
      state.errorByDir.set(path, error.message);
      if (path === "") state.rootError = error.message;
    } finally {
      if (token === revision) {
        state.loadingDirs.delete(path);
        if (path === "") state.rootLoading = false;
        changed();
      }
    }
  }

  function toggle(path) {
    const opening = !state.expanded.has(path);
    if (opening) state.expanded.add(path);
    else state.expanded.delete(path);
    if (opening && !state.childrenByDir.has(path) && !state.loadingDirs.has(path))
      void loadDirectory(path, revision);
    else changed();
  }

  function reset() {
    controller.abort();
    controller = new AbortController();
    const token = ++revision;
    state.rootEntries = [];
    state.rootError = "";
    state.expanded.clear();
    state.loadingDirs.clear();
    state.childrenByDir.clear();
    state.errorByDir.clear();
    state.truncatedDirs.clear();
    state.query = "";
    runSearch();
    void loadDirectory("", token);
  }

  function runSearch() {
    clearTimeout(searchTimer);
    searchController?.abort();
    const query = state.query.trim();
    if (query.length < 2) {
      state.searchResults = null;
      state.searchError = "";
      state.searching = false;
      changed();
      return;
    }
    state.searching = true;
    changed();
    searchController = new AbortController();
    const { signal } = searchController;
    searchTimer = setTimeout(async () => {
      try {
        const result = await api.search(query, signal);
        state.searchResults = result.entries || [];
        state.searchTruncated = Boolean(result.truncated);
        state.searchError = "";
      } catch (error) {
        if (error.name === "AbortError") return;
        state.searchResults = [];
        state.searchError = error.message;
      }
      state.searching = false;
      changed();
    }, SEARCH_DEBOUNCE_MS);
  }

  return {
    state,
    subtitle,
    reset,
    toggle,
    search(query) {
      state.query = query;
      runSearch();
    },
    showBlocked(path, url) {
      state.blocked = { path, url };
      changed();
    },
    clearBlocked() {
      if (!state.blocked) return;
      state.blocked = null;
      changed();
    },
    dismissBlocked() {
      state.blocked = null;
      // Let the link's default navigation finish before removing its node.
      queueMicrotask(changed);
    },
  };
}

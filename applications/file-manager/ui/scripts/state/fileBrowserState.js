/** Owns browser data and its transitions; requests belong to the service. */
export function createFileBrowserState(changed) {
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
    archiving: new Set(),
    archiveError: "",
    searchResults: null,
    searchTruncated: false,
    searching: false,
    searchError: "",
  };

  function subtitle() {
    if (state.searchResults) {
      const count = state.searchResults.length;
      return `${count} result${count === 1 ? "" : "s"}${state.searchTruncated ? "+" : ""}`;
    }
    return state.rootLoading ? "Loading..." : "workspace";
  }

  return {
    state,
    subtitle,
    directoryLoading(path) {
      state.loadingDirs.add(path);
      if (path === "") state.rootLoading = true;
      changed();
    },
    directoryLoaded(path, result) {
      const list = result.entries || [];
      if (path === "") state.rootEntries = list;
      state.childrenByDir.set(path, list);
      state.errorByDir.delete(path);
      if (path === "") state.rootError = "";
      if (result.truncated) state.truncatedDirs.add(path);
      else state.truncatedDirs.delete(path);
    },
    directoryFailed(path, error) {
      state.errorByDir.set(path, error.message);
      if (path === "") state.rootError = error.message;
    },
    directoryFinished(path) {
      state.loadingDirs.delete(path);
      if (path === "") state.rootLoading = false;
      changed();
    },
    toggleDirectory(path) {
      const opening = !state.expanded.has(path);
      if (opening) state.expanded.add(path);
      else state.expanded.delete(path);
      return opening;
    },
    resetTree() {
      state.rootEntries = [];
      state.rootError = "";
      state.expanded.clear();
      state.loadingDirs.clear();
      state.childrenByDir.clear();
      state.errorByDir.clear();
      state.truncatedDirs.clear();
      state.query = "";
    },
    searchQuery(query) {
      state.query = query;
    },
    searchCleared() {
      state.searchResults = null;
      state.searchError = "";
      state.searching = false;
      changed();
    },
    searchStarted() {
      state.searching = true;
      changed();
    },
    searchLoaded(result) {
      state.searchResults = result.entries || [];
      state.searchTruncated = Boolean(result.truncated);
      state.searchError = "";
    },
    searchFailed(error) {
      state.searchResults = [];
      state.searchError = error.message;
    },
    searchFinished() {
      state.searching = false;
      changed();
    },
    archiveStarted(path) {
      state.archiving.add(path);
      state.archiveError = "";
      changed();
    },
    archiveFailed(error) {
      state.archiveError = error.message;
    },
    archiveFinished(path) {
      state.archiving.delete(path);
      changed();
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
    notify: changed,
  };
}

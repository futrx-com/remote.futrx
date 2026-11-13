import { SEARCH_DEBOUNCE_MS } from "../config/files.js";

/** Coordinates requests and cancellation without owning presentation data. */
export function createFileBrowser(api, store, download) {
  const { state } = store;
  let disposed = false;
  let revision = 0;
  let controller = new AbortController();
  let searchTimer;
  let searchController;

  async function loadDirectory(path, token) {
    store.directoryLoading(path);
    try {
      const result = await api.list(path, controller.signal);
      if (token !== revision) return;
      store.directoryLoaded(path, result);
    } catch (error) {
      if (error.name === "AbortError" || token !== revision) return;
      store.directoryFailed(path, error);
    } finally {
      if (token === revision) store.directoryFinished(path);
    }
  }

  function toggle(path) {
    const opening = store.toggleDirectory(path);
    if (opening && !state.childrenByDir.has(path) && !state.loadingDirs.has(path))
      void loadDirectory(path, revision);
    else store.notify();
  }

  // The ZIP is built before the browser is sent to it, so a refused folder
  // shows its reason here instead of a failed download. A ZIP that finishes
  // after the drawer closed still downloads: the user asked for it.
  async function downloadFolder(path) {
    if (state.archiving.has(path)) return;
    store.archiveStarted(path);
    try {
      download(await api.archive(path));
    } catch (error) {
      if (!disposed) store.archiveFailed(error);
    } finally {
      if (!disposed) store.archiveFinished(path);
    }
  }

  function reset() {
    controller.abort();
    controller = new AbortController();
    const token = ++revision;
    store.resetTree();
    runSearch();
    void loadDirectory("", token);
  }

  function runSearch() {
    clearTimeout(searchTimer);
    searchController?.abort();
    const query = state.query.trim();
    if (query.length < 2) {
      store.searchCleared();
      return;
    }
    store.searchStarted();
    searchController = new AbortController();
    const { signal } = searchController;
    searchTimer = setTimeout(async () => {
      try {
        const result = await api.search(query, signal);
        store.searchLoaded(result);
      } catch (error) {
        if (error.name === "AbortError") return;
        store.searchFailed(error);
      }
      store.searchFinished();
    }, SEARCH_DEBOUNCE_MS);
  }

  return {
    state,
    subtitle: store.subtitle,
    reset,
    toggle,
    downloadFolder,
    search(query) {
      store.searchQuery(query);
      runSearch();
    },
    showBlocked: store.showBlocked,
    clearBlocked: store.clearBlocked,
    dismissBlocked: store.dismissBlocked,
    dispose() {
      disposed = true;
      revision++;
      controller.abort();
      clearTimeout(searchTimer);
      searchController?.abort();
    },
  };
}

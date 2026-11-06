import { ROOT_PREFETCH_MAX_AGE_MS } from "../config/files.js";

// Fetches a project's workspace root while its chat is showing, so the Files
// drawer has its tree the moment it finishes opening. Each listing is used by
// at most one drawer, and a project is fetched at most once per window.
export function createRootPrefetch(load, { maxAgeMs = ROOT_PREFETCH_MAX_AGE_MS, now = Date.now } = {}) {
  const listings = new Map();

  return {
    warm(projectId) {
      const current = listings.get(projectId);
      if (current && now() - current.at < maxAgeMs) return;
      const promise = load(projectId);
      // Keep failures quiet until a drawer consumes the promise.
      promise.catch(() => {});
      listings.set(projectId, { at: now(), promise, taken: false });
    },
    /** The recent root listing for projectId, or null. */
    take(projectId) {
      const current = listings.get(projectId);
      if (!current || current.taken || now() - current.at >= maxAgeMs) return null;
      current.taken = true;
      return current.promise;
    },
  };
}

/** Apply the one-use root listing without teaching the backend API about caching. */
export function withPrefetchedRoot(files, prefetched, projectId) {
  return {
    ...files,
    list(path, signal) {
      // Refresh and nested listings always reach the backend after the first use.
      const ready = path ? null : prefetched.take(projectId);
      return ready ?? files.list(path, signal);
    },
  };
}

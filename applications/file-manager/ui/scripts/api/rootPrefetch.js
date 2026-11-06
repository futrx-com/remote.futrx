// Fetches a project's workspace root while its chat is showing, so the Files
// drawer has its tree the moment it finishes opening. Each listing is used by
// at most one drawer, and a project is fetched at most once per window.
export function createRootPrefetch(load, { maxAgeMs = 15000, now = Date.now } = {}) {
  const listings = new Map();

  return {
    warm(projectId) {
      const current = listings.get(projectId);
      if (current && now() - current.at < maxAgeMs) return;
      const promise = load(projectId);
      // A failed prefetch is simply not used; the drawer fetches again.
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

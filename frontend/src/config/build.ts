/**
 * How an open page notices that the server now serves a newer frontend.
 *
 * Every production build is stamped with a fingerprint of its index.html,
 * which names every hashed asset the page loads, so the stamp changes exactly
 * when the frontend does and a backend-only release leaves it alone. The page
 * carries its own stamp in a meta tag; the server publishes the stamp of the
 * build it serves now as a manifest. `vite.config.ts` writes both, which is
 * why it imports these names rather than repeating them.
 */
export const FRONTEND_BUILD = {
  metaName: "remote-build",
  manifestFile: "build.json",
  manifestUrl: "/build.json",
  /** How often a visible page asks whether a newer build is being served. */
  checkIntervalMs: 60_000,
} as const;

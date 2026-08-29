/**
 * Every key this app owns in the browser's localStorage, in one place.
 *
 * These hold per-browser conveniences only — the authoritative copy of anything
 * that matters lives on the server. Listing them together is what makes the
 * namespace auditable, and `themeChoice` in particular is also read by the
 * bootstrap script in `index.html`, which cannot import from here: that literal
 * and this constant have to stay in step.
 */
export const STORAGE_KEYS = {
  themeChoice: "remote.futrx.theme",
  sidebarCollapsed: "remote.futrx.sidebarCollapsed",
  collapsedProjects: "remote.futrx.collapsedProjects",
  workspaceBoot: "remote.futrx.workspaceBoot",
  pushOptIn: "remote.futrx.pushOptIn",
} as const;

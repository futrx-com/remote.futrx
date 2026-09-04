/**
 * Every key this app owns in browser storage, in one place.
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
  searchFilters: "remote.futrx.searchFilters",
  searchSort: "remote.futrx.searchSort",
} as const;

/**
 * Keys in sessionStorage rather than localStorage. Kept separate because the
 * lifetime is the point: these hold per-tab working state that must not leak
 * between tabs or outlive the browser session.
 */
export const SESSION_STORAGE_KEYS = {
  composerSession: "remote.futrx.composerSession.v1",
} as const;

/**
 * Every key this app owns in browser storage, in one place.
 *
 * These hold browser-scoped preferences and consent state. Shared application
 * records remain authoritative on the server; values whose meaning is local to
 * this browser intentionally live here. Listing them together is what makes the
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
  /** The served frontend build this tab last reloaded for; stops a loop when
   *  something between browser and server keeps handing back an older page. */
  frontendBuildReload: "remote.futrx.frontendBuildReload",
  /** The build this tab last reloaded because a diagram chunk failed to load;
   *  a second failure on the same build shows the fallback instead of looping. */
  mermaidChunkReload: "remote.futrx.mermaidChunkReload",
} as const;

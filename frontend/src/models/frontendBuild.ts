/** What the server publishes about the frontend build it serves now. */
export interface FrontendBuildManifest {
  build: string;
}

export interface FrontendBuildStoreState {
  /** The build the server last said it serves; null until it has answered. */
  served: string | null;
}

export interface FrontendBuildStoreActions {
  /** Asks the server which build it serves. Overlapping calls share a request. */
  check: () => Promise<void>;
}

/** Everything the reload decision reads about the page at one moment. */
export interface FrontendBuildPage {
  /** The build this page was loaded from; null for the Vite dev server. */
  running: string | null;
  served: string | null;
  /** The served build this tab already reloaded for, if any. */
  reloadedFor: string | null;
}

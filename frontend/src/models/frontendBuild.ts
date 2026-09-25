/** What the server publishes about the frontend build it serves now. */
export interface FrontendBuildManifest {
  build: string;
}

export interface FrontendBuildStoreState {
  /** The build the server last said it serves; null until it has answered. */
  served: string | null;
  /** How many surfaces hold work that a reload would lose. */
  holds: number;
}

export interface FrontendBuildStoreActions {
  /** Asks the server which build it serves. Overlapping calls share a request. */
  check: () => Promise<void>;
  /** Keeps the page from reloading onto a newer build until the release runs. */
  hold: () => () => void;
}

/** Everything the reload decision reads about the page at one moment. */
export interface FrontendBuildPage {
  /** The build this page was loaded from; null for the Vite dev server. */
  running: string | null;
  served: string | null;
  /** The served build this tab already reloaded for, if any. */
  reloadedFor: string | null;
  /** The user is just coming back to the page, before touching anything. */
  opening: boolean;
  hidden: boolean;
  /** Typing in a field, or a modal dialog is open. */
  editing: boolean;
  /** Something registered work that only lives in this page, such as uploads. */
  held: boolean;
}

export type FrontendBuildReload = "stay" | "wait" | "reload";

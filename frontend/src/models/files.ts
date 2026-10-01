export interface FileNode {
  name: string;
  /** Path relative to the workspace root, forward slashes (used by download URLs). */
  path: string;
  isDir: boolean;
  size?: number;
  modTime?: number;
}

export interface FileOpenRequest {
  cwd: string;
  path: string;
  line?: number;
  column?: number;
}

export interface DirListing {
  /** The directory that was listed ("" = workspace root). */
  path: string;
  entries: FileNode[];
  /** True when the directory hit the per-listing entry cap and is partial. */
  truncated: boolean;
}

export interface FileSearchResult {
  entries: FileNode[];
  /** True when the search hit its result/visit cap and is partial. */
  truncated: boolean;
}

/** What the in-app viewer can render inline, mirroring the backend's
 *  workspacefiles mediaTypes. */
export type MediaKind = "image" | "video" | "audio" | "pdf";

/** The broad kind of a file, picked from its extension — what the file tree
 *  chooses an icon and a colour by. */
export type FileCategory =
  | "image"
  | "video"
  | "audio"
  | "pdf"
  | "archive"
  | "code"
  | "data"
  | "text";

export interface MediaViewerItem {
  url: string;
  name: string;
  kind: MediaKind;
}

export interface MediaViewerStoreState {
  item: MediaViewerItem | null;
}

export interface MediaViewerStoreActions {
  open: (item: MediaViewerItem) => void;
  close: () => void;
}

export interface WorkspaceFileBrowserState {
  childrenByDir: Map<string, FileNode[]>;
  expanded: Set<string>;
  loading: Set<string>;
  errorByDir: Map<string, string>;
  truncatedDirs: Set<string>;
  rootLoading: boolean;
  query: string;
  searchResults: FileNode[] | null;
  searchTruncated: boolean;
  searching: boolean;
  searchError: string | null;
}

export type WorkspaceFileBrowserAction =
  | { type: "reset" }
  | { type: "directory-load-started"; path: string }
  | { type: "directory-load-succeeded"; path: string; entries: FileNode[]; truncated: boolean }
  | { type: "directory-load-failed"; path: string; error: string }
  | { type: "directory-toggled"; path: string }
  | { type: "query-changed"; query: string }
  | { type: "search-idle" }
  | { type: "search-started" }
  | { type: "search-succeeded"; entries: FileNode[]; truncated: boolean }
  | { type: "search-failed"; error: string };

export type FileOpener = (request: FileOpenRequest) => string | null;

export interface FileOpenerEntry {
  open: FileOpener;
  projectIds: string[];
}

export interface FileOpenerStoreState {
  revision: number;
  register(applicationId: string, projectIds: string[], open: FileOpener): () => void;
  setProjects(applicationId: string, projectIds: string[]): void;
  remove(applicationId: string): void;
  canOpen(projectId?: string): boolean;
  forProject(projectId?: string): IterableIterator<FileOpener>;
}

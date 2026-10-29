export interface FileOpenRequest {
  /** Set by the opener registry from the project selecting the editor. */
  projectId?: string;
  cwd: string;
  path: string;
  line?: number;
  column?: number;
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

/** What opening a file did. `blocked` means the browser refused the editor
 *  tab; the caller can offer the editor URL as a link instead. */
export type FileOpenResult = "media" | "editor" | "blocked" | "download" | "ignored";

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

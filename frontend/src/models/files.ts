/** What the in-app viewer can render inline, mirroring the backend's
 *  workspacefiles mediaTypes. */
export type MediaKind = "image" | "video" | "audio" | "pdf";

/** One item displayed by the app-wide media viewer. */
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

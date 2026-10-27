import { createStore } from "zustand/vanilla";
import type {
  MediaViewerStoreActions,
  MediaViewerStoreState,
} from "../../../models/files";

// App-wide in-app media viewer. Any surface (file manager rows, chat message
// links) opens media here instead of navigating away; a single overlay host
// subscribes and renders the current item.
export const mediaViewerStore = createStore<
  MediaViewerStoreState & MediaViewerStoreActions
>()((set) => ({
  item: null,
  open: (item) => set({ item }),
  close: () => set((state) => state.item === null ? state : { item: null }),
}));

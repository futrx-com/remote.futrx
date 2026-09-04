import { createStore } from "zustand/vanilla";
import type {
  CommandPaletteStoreActions,
  CommandPaletteStoreState,
} from "../../../models/search";

// Whether the command palette is on screen. Global because the surfaces that
// open it and the one that renders it are on opposite sides of the workspace
// shell: the sidebar's search box opens it, the chord toggles it from the
// window, and nothing between them owns both.
//
// The palette's own search selection is separate and lives in
// `paletteSearchStore` -- dismissing the palette does not clear it, and holding
// them together would make visibility and selection re-render each other.
export const commandPaletteStore = createStore<
  CommandPaletteStoreState & CommandPaletteStoreActions
>()((set) => ({
  open: false,
  openPalette: () => set((state) => state.open ? state : { open: true }),
  closePalette: () => set((state) => state.open ? { open: false } : state),
  togglePalette: () => set((state) => ({ open: !state.open })),
}));

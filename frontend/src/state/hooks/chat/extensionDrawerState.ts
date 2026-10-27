import { EXTENSION_DRAWER_WIDTH } from "../../../config/extensions.ts";

export interface ExtensionDrawerWidthLimits {
  defaultWidth: number;
  minWidth: number;
}

export const extensionDrawerState = {
  /** Name the chat view knows an open drawer by, beside "files", "history", "browser". */
  paneName: (drawerId: string) => `extension-${drawerId}`,

  paneElementId: (drawerId: string) => `workspace-extension-${drawerId}-pane`,

  widthStorageKey: (drawerId: string) => `remote.futrx.extensionDrawerWidth.${drawerId}`,

  /** Widest the drawer may be in a container, always leaving room for the chat. */
  availableWidth(containerWidth: number, minWidth: number): number {
    return Math.min(
      EXTENSION_DRAWER_WIDTH.max,
      Math.max(minWidth, containerWidth - EXTENSION_DRAWER_WIDTH.minChat),
    );
  },

  clampWidth(
    width: number,
    minWidth: number,
    maxWidth: number = EXTENSION_DRAWER_WIDTH.max,
  ): number {
    return Math.min(Math.max(width, minWidth), Math.max(minWidth, maxWidth));
  },

  /** Width to start from, given whatever localStorage holds for the drawer. */
  storedWidth(stored: string | null, limits: ExtensionDrawerWidthLimits): number {
    const width = Number(stored);
    return Number.isFinite(width) && width > 0
      ? extensionDrawerState.clampWidth(width, limits.minWidth)
      : limits.defaultWidth;
  },
};

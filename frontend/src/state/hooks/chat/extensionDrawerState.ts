import { EXTENSION_DRAWER_WIDTH } from "../../../config/extensions.ts";
import type { ExtensionDrawerAction } from "../../../models/extension.ts";

const ACTION_ID = /^[a-z0-9-]+$/;

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

  /** Header buttons an application asked for, minus malformed or repeated ones. */
  validActions(actions: ExtensionDrawerAction[] | null): ExtensionDrawerAction[] {
    if (!Array.isArray(actions)) return [];
    const seen = new Set<string>();
    const valid: ExtensionDrawerAction[] = [];
    for (const action of actions) {
      if (
        !action ||
        typeof action.id !== "string" ||
        !ACTION_ID.test(action.id) ||
        seen.has(action.id) ||
        typeof action.label !== "string" ||
        typeof action.icon !== "string" ||
        typeof action.onClick !== "function"
      ) {
        console.error("[extensions] invalid drawer action", action);
        continue;
      }
      seen.add(action.id);
      valid.push({
        id: action.id,
        label: action.label,
        icon: action.icon,
        onClick: action.onClick,
        disabled: Boolean(action.disabled),
        busy: Boolean(action.busy),
      });
    }
    return valid;
  },
};

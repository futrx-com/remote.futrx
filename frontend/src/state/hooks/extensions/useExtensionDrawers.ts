import { useStore } from "zustand";
import { useMemo } from "preact/hooks";
import type { ExtensionDrawerTarget } from "../../../models/extension";
import { extensionStore } from "../../stores/extensions/extensionStore";
import { visibleExtensionDrawers } from "./extensionContributionState";

export function useExtensionDrawers({ chatId, projectId, cwd }: ExtensionDrawerTarget) {
  const drawers = useStore(extensionStore, (state) => state.drawers);
  const activeProjectId = useStore(
    extensionStore,
    (state) => state.activeProjectId,
  );
  return useMemo(
    () => visibleExtensionDrawers(drawers, { chatId, projectId, cwd }, activeProjectId),
    [drawers, chatId, projectId, cwd, activeProjectId],
  );
}

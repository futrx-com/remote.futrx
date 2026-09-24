import { useMemo } from "preact/hooks";
import { useStore } from "zustand";
import type { ExtensionWorkspacePaneContext } from "../../../models/extension";
import { extensionStore } from "../../stores/extensions/extensionStore";
import { visibleExtensionWorkspacePanes } from "./extensionContributionState";

export function useExtensionWorkspacePanes(
  context: ExtensionWorkspacePaneContext,
) {
  const panes = useStore(extensionStore, (state) => state.workspacePanes);
  const activeProjectId = useStore(
    extensionStore,
    (state) => state.activeProjectId,
  );
  return useMemo(
    () => visibleExtensionWorkspacePanes(panes, context, activeProjectId),
    [panes, context, activeProjectId],
  );
}

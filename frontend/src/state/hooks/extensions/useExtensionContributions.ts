import { useStore } from "zustand";
import { useMemo } from "preact/hooks";
import type {
  ExtensionContribution,
  ExtensionSlotContext,
  ExtensionSlotName,
} from "../../../models/extension";
import { extensionStore } from "../../stores/extensions/extensionStore";
import { visibleExtensionContributions } from "./extensionContributionState";

const NO_CONTRIBUTIONS: ExtensionContribution[] = [];

export function useExtensionContributions(
  slot: ExtensionSlotName,
  context: ExtensionSlotContext,
) {
  const contributions = useStore(
    extensionStore,
    (state) => state.bySlot.get(slot) ?? NO_CONTRIBUTIONS,
  );
  const activeProjectId = useStore(
    extensionStore,
    (state) => state.activeProjectId,
  );
  return useMemo(
    () => visibleExtensionContributions(contributions, context, activeProjectId),
    [contributions, context, activeProjectId],
  );
}

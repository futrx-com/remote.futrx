import type {
  ExtensionContribution,
  ExtensionDrawerContribution,
  ExtensionDrawerTarget,
  ExtensionSlotContext,
  ExtensionVisibility,
} from "../../../models/extension.ts";

export function visibleExtensionContributions(
  contributions: ExtensionContribution[],
  context: ExtensionSlotContext,
  activeProjectId: string | null,
): ExtensionContribution[] {
  return contributions.filter((contribution) => {
    if (!isInScope(contribution, context, activeProjectId)) return false;
    if (!contribution.when) return true;
    try {
      return contribution.when(context);
    } catch (error) {
      console.error(
        `[extensions] ${contribution.applicationId}: predicate failed`,
        error,
      );
      return false;
    }
  });
}

/** Drawers a chat should offer: in install scope, and wanted by their `when`. */
export function visibleExtensionDrawers(
  drawers: ExtensionDrawerContribution[],
  target: ExtensionDrawerTarget,
  activeProjectId: string | null,
): ExtensionDrawerContribution[] {
  return drawers.filter((drawer) => {
    if (!isInScope(drawer, target, activeProjectId)) return false;
    if (!drawer.when) return true;
    try {
      return drawer.when(target);
    } catch (error) {
      console.error(`[extensions] ${drawer.applicationId}: drawer predicate failed`, error);
      return false;
    }
  });
}

function isInScope(
  contribution: { visibility: ExtensionVisibility },
  context: Pick<ExtensionSlotContext, "scope" | "projectId">,
  activeProjectId: string | null,
): boolean {
  const { global, projectIds } = contribution.visibility;
  if (global) return true;
  if (context.scope === "global") return false;
  if (activeProjectId === null || !projectIds.includes(activeProjectId)) {
    return false;
  }
  return context.projectId === undefined || projectIds.includes(context.projectId);
}

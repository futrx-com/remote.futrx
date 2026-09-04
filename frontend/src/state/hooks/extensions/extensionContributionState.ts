import type {
  ExtensionContribution,
  ExtensionSlotContext,
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
        `[extensions] ${contribution.imageId}: predicate failed`,
        error,
      );
      return false;
    }
  });
}

function isInScope(
  contribution: ExtensionContribution,
  context: ExtensionSlotContext,
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

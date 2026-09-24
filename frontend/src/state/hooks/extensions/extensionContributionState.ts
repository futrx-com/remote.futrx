import type {
  ExtensionContribution,
  ExtensionSlotContext,
  ExtensionWorkspacePaneContext,
  ExtensionWorkspacePaneContribution,
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

export function visibleExtensionWorkspacePanes(
  panes: ExtensionWorkspacePaneContribution[],
  context: ExtensionWorkspacePaneContext,
  activeProjectId: string | null,
): ExtensionWorkspacePaneContribution[] {
  return panes.filter((pane) => {
    if (!isVisibilityInScope(pane.visibility, context.projectId, activeProjectId)) {
      return false;
    }
    if (!pane.when) return true;
    try {
      return pane.when(context);
    } catch (error) {
      console.error(
        `[extensions] ${pane.applicationId}: workspace pane predicate failed`,
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
  return isVisibilityInScope(
    { global, projectIds },
    context.projectId,
    activeProjectId,
  );
}

function isVisibilityInScope(
  visibility: { global: boolean; projectIds: string[] },
  projectId: string | undefined,
  activeProjectId: string | null,
): boolean {
  const { global, projectIds } = visibility;
  if (global) return true;
  if (activeProjectId === null || !projectIds.includes(activeProjectId)) {
    return false;
  }
  return projectId === undefined || projectIds.includes(projectId);
}

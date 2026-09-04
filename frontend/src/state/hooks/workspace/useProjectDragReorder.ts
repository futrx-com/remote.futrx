import { useState } from "preact/hooks";
import type { DropPosition } from "../../../models/workspace";
import { workspaceSidebarService } from "../../../services/workspace/workspaceSidebarService.ts";

interface DropTarget {
  projectId: string;
  position: DropPosition;
}

/**
 * Drag-to-reorder for the sidebar's project groups: which group is in flight,
 * which gap the drop marker sits in, and the reorder that lands.
 *
 * The sidebar only has to draw what this reports — the pointer arithmetic and
 * the "did anything actually move" question both live here.
 */
export function useProjectDragReorder({
  projectIds,
  enabled,
  onReorder,
}: {
  projectIds: string[];
  enabled: boolean;
  onReorder: (projectIds: string[]) => void;
}) {
  const [dragProjectId, setDragProjectId] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<DropTarget | null>(null);

  // Which side of the hovered group the project would land on. Groups differ
  // wildly in height once their chats are expanded, so the split is the
  // hovered group's own midpoint rather than a fixed edge band.
  function dropPositionFor(event: DragEvent): DropPosition {
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    return event.clientY < rect.top + rect.height / 2 ? "before" : "after";
  }

  function endDrag() {
    setDragProjectId(null);
    setDropTarget(null);
  }

  return {
    isDragging(projectId: string): boolean {
      return dragProjectId === projectId;
    },

    /** Where the marker goes for this group, or null when it is not the target. */
    dropPositionOf(projectId: string): DropPosition | null {
      if (dropTarget?.projectId !== projectId || dragProjectId === projectId) return null;
      return dropTarget.position;
    },

    handlersFor(projectId: string) {
      return {
        onDragStart(event: DragEvent) {
          if (!enabled) return;
          setDragProjectId(projectId);
          event.dataTransfer?.setData("text/plain", projectId);
          if (event.dataTransfer) event.dataTransfer.effectAllowed = "move";
        },
        onDragOver(event: DragEvent) {
          if (!enabled || !dragProjectId) return;
          event.preventDefault();
          if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
          setDropTarget({ projectId, position: dropPositionFor(event) });
        },
        onDragLeave(event: DragEvent) {
          // dragleave also fires when crossing into a child, so only drop the
          // marker once the pointer has actually left the group.
          const entering = event.relatedTarget as Node | null;
          if (entering && (event.currentTarget as HTMLElement).contains(entering)) return;
          setDropTarget((current) => (current?.projectId === projectId ? null : current));
        },
        onDrop(event: DragEvent) {
          if (!enabled) return;
          event.preventDefault();
          const sourceId = dragProjectId || event.dataTransfer?.getData("text/plain") || "";
          const next = workspaceSidebarService.reorderProjectIds(
            projectIds,
            sourceId,
            projectId,
            dropPositionFor(event)
          );
          if (next) onReorder(next);
          endDrag();
        },
        onDragEnd: endDrag,
      };
    },
  };
}

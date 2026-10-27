import type { ChatMeta } from "../../models/chat.ts";
import type { ProjectMeta } from "../../models/project.ts";
import type { DropPosition, WorkspaceSidebarModel } from "../../models/workspace.ts";

interface ChatBuckets {
  byProject: Map<string, ChatMeta[]>;
  loose: ChatMeta[];
}

class WorkspaceSidebarService {
  activeChat(chats: ChatMeta[], activeChatId: string | null): ChatMeta | null {
    return activeChatId ? chats.find((chat) => chat.id === activeChatId) ?? null : null;
  }

  initialChatId(
    gateOpen: boolean,
    activeChatId: string | null,
    chats: ChatMeta[]
  ): string | null {
    if (!gateOpen || activeChatId !== null || chats.length === 0) return null;
    return chats[0].id;
  }

  isActiveChatMissing(chats: ChatMeta[], activeChatId: string | null): boolean {
    return !!activeChatId && !chats.some((chat) => chat.id === activeChatId);
  }

  /** Who takes over when the active chat disappears. Same pick as
   *  `initialChatId`, so a deletion lands where a fresh load would. */
  replacementChatId(chats: ChatMeta[]): string | null {
    return chats.length > 0 ? chats[0].id : null;
  }

  /** Drag-reorder result, or null when the drop would not move anything.
   *  Removing before inserting is what keeps a downward move honest: splicing
   *  at the target's original index drops the project one slot short. */
  reorderProjectIds(
    ids: string[],
    sourceId: string,
    targetId: string,
    position: DropPosition
  ): string[] | null {
    if (!ids.includes(sourceId) || !ids.includes(targetId)) return null;
    const without = ids.filter((id) => id !== sourceId);
    const targetIndex = without.indexOf(targetId);
    if (targetIndex < 0) return null;
    const next = without.slice();
    next.splice(position === "before" ? targetIndex : targetIndex + 1, 0, sourceId);
    return next.every((id, index) => id === ids[index]) ? null : next;
  }

  /** Group chats under their projects, in the sidebar's display order. */
  model(chats: ChatMeta[], projects: ProjectMeta[]): WorkspaceSidebarModel {
    const buckets = this.bucketChatsByProject(chats, new Set(projects.map((p) => p.id)));
    const visibleProjects = [...projects]
      .sort((left, right) => this.compareProjects(left, right))
      .map((project) => ({
        project,
        chats: buckets.byProject.get(project.id) ?? [],
      }));

    return {
      visibleProjects,
      visibleLooseChats: buckets.loose,
      totalChats: chats.length,
      totalProjects: projects.length,
    };
  }

  /** `knownProjectIds` is what keeps a chat whose project was deleted visible:
   *  bucketed under a project that is never rendered, it would vanish from the
   *  sidebar entirely, so a dangling id counts as no project at all. */
  private bucketChatsByProject(
    chats: ChatMeta[],
    knownProjectIds: ReadonlySet<string>
  ): ChatBuckets {
    const byProject = new Map<string, ChatMeta[]>();
    const loose: ChatMeta[] = [];

    for (const chat of chats) {
      if (!chat.projectId || !knownProjectIds.has(chat.projectId)) {
        loose.push(chat);
        continue;
      }

      const projectChats = byProject.get(chat.projectId) ?? [];
      projectChats.push(chat);
      byProject.set(chat.projectId, projectChats);
    }

    for (const projectChats of byProject.values()) {
      projectChats.sort((left, right) => right.lastMessageAt - left.lastMessageAt);
    }
    loose.sort((left, right) => right.lastMessageAt - left.lastMessageAt);

    return { byProject, loose };
  }

  private compareProjects(left: ProjectMeta, right: ProjectMeta): number {
    const leftOrder = left.order || left.createdAt || 0;
    const rightOrder = right.order || right.createdAt || 0;
    if (leftOrder !== rightOrder) return rightOrder - leftOrder;
    return right.updatedAt - left.updatedAt;
  }
}

export const workspaceSidebarService = new WorkspaceSidebarService();

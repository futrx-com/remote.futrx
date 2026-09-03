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

  model(chats: ChatMeta[], projects: ProjectMeta[], rawQuery: string): WorkspaceSidebarModel {
    const query = rawQuery.trim().toLowerCase();
    const buckets = this.bucketChatsByProject(chats);
    const sortedProjects = [...projects].sort((left, right) => this.compareProjects(left, right));

    const visibleProjects = sortedProjects
      .map((project) => {
        const projectChats = buckets.byProject.get(project.id) ?? [];
        const projectMatches = this.matchesProject(project, query);
        const filteredChats = query
          ? projectChats.filter((chat) => projectMatches || this.matchesChat(chat, query))
          : projectChats;
        return { project, chats: projectChats, filteredChats };
      })
      .filter(
        (node) =>
          !query || this.matchesProject(node.project, query) || node.filteredChats.length > 0
      );

    const visibleLooseChats = buckets.loose.filter((chat) => this.matchesChat(chat, query));

    return {
      visibleProjects,
      visibleLooseChats,
      totalChats: chats.length,
      totalProjects: projects.length,
      hasMatches: visibleProjects.length > 0 || visibleLooseChats.length > 0,
      query,
    };
  }

  private bucketChatsByProject(chats: ChatMeta[]): ChatBuckets {
    const byProject = new Map<string, ChatMeta[]>();
    const loose: ChatMeta[] = [];

    for (const chat of chats) {
      if (!chat.projectId) {
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

  private matchesChat(chat: ChatMeta, query: string): boolean {
    if (!query) return true;
    return `${chat.title} ${chat.cwd ?? ""} ${chat.model ?? ""}`
      .toLowerCase()
      .includes(query);
  }

  private matchesProject(project: ProjectMeta, query: string): boolean {
    if (!query) return true;
    return `${project.name} ${project.slug}`.toLowerCase().includes(query);
  }

  private compareProjects(left: ProjectMeta, right: ProjectMeta): number {
    const leftOrder = left.order || left.createdAt || 0;
    const rightOrder = right.order || right.createdAt || 0;
    if (leftOrder !== rightOrder) return rightOrder - leftOrder;
    return right.updatedAt - left.updatedAt;
  }
}

export const workspaceSidebarService = new WorkspaceSidebarService();

import type { ChatMeta } from "../../models/chat.ts";
import type { ProjectMeta } from "../../models/project.ts";
import { STORAGE_KEYS } from "../../config/storageKeys.ts";
import { browserStorageService } from "../platform/browserStorageService.ts";

// What the user folded away in the sidebar, and what to fold on first sight.
// Split from workspaceSidebarService because it changes for a different
// reason: the storage keys and their shape, not the sidebar's layout rules.
class SidebarPreferenceService {
  readCollapsed(): boolean {
    return browserStorageService.readBool(STORAGE_KEYS.sidebarCollapsed);
  }

  writeCollapsed(collapsed: boolean): void {
    browserStorageService.writeBool(STORAGE_KEYS.sidebarCollapsed, collapsed);
  }

  /** Which projects the user left folded, remembered across reloads. Projects
   *  missing from the map stay out of it, so `seedCollapsedProjects` can still
   *  seed them from unread state the first time they show up. */
  readCollapsedProjects(): Record<string, boolean> {
    const parsed = browserStorageService.readJson(STORAGE_KEYS.collapsedProjects);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};
    const collapsed: Record<string, boolean> = {};
    for (const [id, value] of Object.entries(parsed as Record<string, unknown>)) {
      if (typeof value === "boolean") collapsed[id] = value;
    }
    return collapsed;
  }

  writeCollapsedProjects(collapsed: Record<string, boolean>): void {
    browserStorageService.writeJson(STORAGE_KEYS.collapsedProjects, collapsed);
  }

  /** Seeds collapse state for projects we have not seen yet; an existing entry is
   *  the user's own choice and survives project/chat churn (e.g. a new chat). */
  seedCollapsedProjects(
    projects: ProjectMeta[],
    chats: ChatMeta[],
    current: Record<string, boolean> = {}
  ): Record<string, boolean> {
    const collapsed: Record<string, boolean> = {};
    for (const project of projects) {
      collapsed[project.id] =
        project.id in current
          ? current[project.id]
          : !this.projectHasUnreadChat(project.id, chats);
    }
    return collapsed;
  }

  hasSameCollapsedProjects(
    current: Record<string, boolean>,
    next: Record<string, boolean>
  ): boolean {
    const currentKeys = Object.keys(current);
    const nextKeys = Object.keys(next);
    if (currentKeys.length !== nextKeys.length) return false;
    return nextKeys.every((key) => current[key] === next[key]);
  }

  private projectHasUnreadChat(projectId: string, chats: ChatMeta[]): boolean {
    return chats.some(
      (chat) =>
        chat.projectId === projectId && (chat.lastMessageAt || 0) > (chat.lastReadAt || 0)
    );
  }
}

export const sidebarPreferenceService = new SidebarPreferenceService();

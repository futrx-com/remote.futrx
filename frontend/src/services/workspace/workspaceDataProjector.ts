import type { ChatMeta, SelectedSkill } from "../../models/chat";
import type { ProjectMeta } from "../../models/project";

class WorkspaceDataProjector {
  replaceChats(next: ChatMeta[], current: ChatMeta[]): ChatMeta[] {
    return this.sortedChats(next, current);
  }

  upsertChat(current: ChatMeta[], chat: ChatMeta): ChatMeta[] {
    return this.sortedChats(this.upsertById(current, chat), current);
  }

  removeChat(current: ChatMeta[], chatId: string): ChatMeta[] {
    return this.sortedChats(current.filter((chat) => chat.id !== chatId), current);
  }

  replaceProjects(next: ProjectMeta[], current: ProjectMeta[]): ProjectMeta[] {
    return this.sortedProjects(next, current);
  }

  upsertProject(current: ProjectMeta[], project: ProjectMeta): ProjectMeta[] {
    return this.sortedProjects(this.upsertById(current, project), current);
  }

  removeProject(current: ProjectMeta[], projectId: string): ProjectMeta[] {
    return this.sortedProjects(
      current.filter((project) => project.id !== projectId),
      current
    );
  }

  private upsertById<T extends { id: string }>(items: T[], item: T): T[] {
    const index = items.findIndex((candidate) => candidate.id === item.id);
    if (index < 0) return [...items, item];
    const next = items.slice();
    next[index] = item;
    return next;
  }

  private sortedChats(next: ChatMeta[], current: ChatMeta[]): ChatMeta[] {
    const sorted = next.slice().sort((left, right) => right.lastMessageAt - left.lastMessageAt);
    return this.sameChats(current, sorted) ? current : sorted;
  }

  private sortedProjects(next: ProjectMeta[], current: ProjectMeta[]): ProjectMeta[] {
    const sorted = next.slice().sort((left, right) => this.compareProjects(left, right));
    return this.sameProjects(current, sorted) ? current : sorted;
  }

  private compareProjects(left: ProjectMeta, right: ProjectMeta): number {
    const leftOrder = left.order || left.createdAt || 0;
    const rightOrder = right.order || right.createdAt || 0;
    if (leftOrder !== rightOrder) return rightOrder - leftOrder;
    return right.createdAt - left.createdAt;
  }

  private sameChats(leftChats: ChatMeta[], rightChats: ChatMeta[]): boolean {
    if (leftChats.length !== rightChats.length) return false;
    for (let index = 0; index < leftChats.length; index++) {
      const left = leftChats[index];
      const right = rightChats[index];
      if (
        left.id !== right.id ||
        left.title !== right.title ||
        left.provider !== right.provider ||
        !this.sameStringRecord(left.sessions, right.sessions) ||
        left.claudeSessionId !== right.claudeSessionId ||
        left.codexSessionId !== right.codexSessionId ||
        left.kimiSessionId !== right.kimiSessionId ||
        left.antigravitySessionId !== right.antigravitySessionId ||
        left.tmuxSession !== right.tmuxSession ||
        left.cwd !== right.cwd ||
        left.createdAt !== right.createdAt ||
        left.lastMessageAt !== right.lastMessageAt ||
        left.lastReadAt !== right.lastReadAt ||
        left.running !== right.running ||
        left.model !== right.model ||
        left.mode !== right.mode ||
        left.reasoningEffort !== right.reasoningEffort ||
        left.serviceTier !== right.serviceTier ||
        left.projectId !== right.projectId ||
        !this.sameSelectedSkills(left.selectedSkills, right.selectedSkills)
      ) {
        return false;
      }
    }
    return true;
  }

  private sameSelectedSkills(
    left: SelectedSkill[] | undefined,
    right: SelectedSkill[] | undefined
  ): boolean {
    if (left === right) return true;
    const leftSkills = left ?? [];
    const rightSkills = right ?? [];
    if (leftSkills.length !== rightSkills.length) return false;

    const key = (skill: SelectedSkill): string => {
      const provider = (skill.provider ?? "").trim().toLowerCase();
      const command = (skill.command ?? skill.name).trim().toLowerCase();
      const source = command === "scheduled-tasks"
        ? "remote"
        : (skill.source ?? "").trim().toLowerCase();
      const name = skill.name.trim();
      return `${provider}:${source}:${command}:${name}`;
    };

    for (let index = 0; index < leftSkills.length; index++) {
      if (key(leftSkills[index]) !== key(rightSkills[index])) return false;
    }
    return true;
  }

  private sameStringRecord(
    left: Record<string, string> | undefined,
    right: Record<string, string> | undefined
  ): boolean {
    if (left === right) return true;
    const leftKeys = Object.keys(left ?? {});
    const rightKeys = Object.keys(right ?? {});
    if (leftKeys.length !== rightKeys.length) return false;
    return leftKeys.every((key) => left?.[key] === right?.[key]);
  }

  private sameProjects(leftProjects: ProjectMeta[], rightProjects: ProjectMeta[]): boolean {
    if (leftProjects.length !== rightProjects.length) return false;
    for (let index = 0; index < leftProjects.length; index++) {
      const left = leftProjects[index];
      const right = rightProjects[index];
      if (
        left.id !== right.id ||
        left.name !== right.name ||
        left.slug !== right.slug ||
        left.cwd !== right.cwd ||
        left.containerName !== right.containerName ||
        left.status !== right.status ||
        left.order !== right.order ||
        left.errorMsg !== right.errorMsg ||
        left.createdAt !== right.createdAt ||
        left.updatedAt !== right.updatedAt
      ) {
        return false;
      }
    }
    return true;
  }
}

export const workspaceDataProjector = new WorkspaceDataProjector();

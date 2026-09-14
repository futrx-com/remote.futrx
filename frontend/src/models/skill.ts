import type { ChatProvider } from "./chat";

export interface RegisteredSkill {
  name: string;
  command?: string;
  description?: string;
  provider: ChatProvider;
  source?: "user" | "system" | "plugin" | "project" | "global" | string;
  // "global" marks an entry served by the platform-wide skills library.
  scope?: "global" | string;
  // alwaysOn marks a global skill an admin pinned into every new chat.
  alwaysOn?: boolean;
  // shadowed marks a global skill hidden behind a same-named project skill.
  shadowed?: boolean;
  // readOnly marks entries a project member cannot edit from the project.
  readOnly?: boolean;
}


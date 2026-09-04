import type { WorkspaceSnapshot } from "../models/workspace";

export const EMPTY_WORKSPACE_SNAPSHOT: WorkspaceSnapshot = {
  chats: [],
  projects: [],
  loaded: false,
};

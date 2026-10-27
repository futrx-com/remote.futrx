import type { WorkspaceSnapshot } from "../models/workspace";

/** Default host workspace used when a chat has no current directory. */
export const DEFAULT_WORKSPACE_PATH = "/opt/remote.futrx";

export const EMPTY_WORKSPACE_SNAPSHOT: WorkspaceSnapshot = {
  chats: [],
  projects: [],
  loaded: false,
};

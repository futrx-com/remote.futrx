import { createStore } from "zustand/vanilla";
import type { ChatMeta } from "../../../models/chat";
import type { ProjectMeta } from "../../../models/project";
import type {
  SubscribeToWorkspace,
  WorkspaceStoreActions,
  WorkspaceStoreState,
} from "../../../models/workspace";
import type { WorkspaceMessage } from "../../../types/workspaceApi";
import { EMPTY_WORKSPACE_SNAPSHOT } from "../../../config/workspace.ts";
import { workspaceDataProjector } from "../../../services/workspace/workspaceDataProjector.ts";

/**
 * The chats and projects the server is pushing, held outside the component
 * tree because the feed is one connection for the whole app rather than one
 * per mount.
 *
 * The snapshot object is rebuilt only when a message actually changes
 * something — the projector returns the same array when it does not — so a
 * subscriber that stores the snapshot re-renders on real changes and not on
 * traffic. Store subscribers are notified on the same condition.
 */
export function createWorkspaceStore(subscribe: SubscribeToWorkspace) {
  // The feed is injected rather than imported: it keeps this module free of the
  // api layer, which is what lets a test drive it with a hand-held feed and
  // what lets the node test runner load it at all.
  let disconnect: (() => void) | undefined;

  return createStore<WorkspaceStoreState & WorkspaceStoreActions>()(
    (set, get) => {
      function commit(chats: ChatMeta[], projects: ProjectMeta[], loaded: boolean): void {
        set((state) => {
          const current = state.snapshot;
          if (chats === current.chats && projects === current.projects && loaded === current.loaded) {
            return state;
          }
          return { snapshot: { chats, projects, loaded } };
        });
      }

      function upsertChat(chat: ChatMeta): void {
        const { chats, projects, loaded } = get().snapshot;
        commit(workspaceDataProjector.upsertChat(chats, chat), projects, loaded);
      }

      function apply(message: WorkspaceMessage): void {
        const { chats, projects, loaded } = get().snapshot;
        switch (message.type) {
          case "workspace.snapshot":
            commit(
              workspaceDataProjector.replaceChats(message.chats, chats),
              workspaceDataProjector.replaceProjects(message.projects, projects),
              true,
            );
            break;
          case "chat.upsert":
            upsertChat(message.chat);
            break;
          case "chat.delete":
            commit(workspaceDataProjector.removeChat(chats, message.id), projects, loaded);
            break;
          case "project.upsert":
            commit(chats, workspaceDataProjector.upsertProject(projects, message.project), loaded);
            break;
          case "project.delete":
            commit(chats, workspaceDataProjector.removeProject(projects, message.id), loaded);
            break;
        }
      }

      return {
        snapshot: EMPTY_WORKSPACE_SNAPSHOT,
        // A chat created or forked from this client exists on the server before
        // its `chat.upsert` reaches us. Seeding it closes that window: without
        // it the freshly selected chat reads as missing from the list and the
        // handover effect bounces the selection back to the chat the user came
        // from. It takes the same door the server's own upsert takes, so the
        // list has one projection path and an early arrival is
        // indistinguishable from the message it beat.
        seedChat: upsertChat,
        /** Opens the feed, or closes it and clears what it delivered. Repeating a
         *  state is a no-op, so callers may drive this from an effect. */
        setConnected: (connected) => {
          if (connected) {
            if (!disconnect) disconnect = subscribe(apply);
            return;
          }
          disconnect?.();
          disconnect = undefined;
          commit(EMPTY_WORKSPACE_SNAPSHOT.chats, EMPTY_WORKSPACE_SNAPSHOT.projects, false);
        },
      };
    },
  );
}

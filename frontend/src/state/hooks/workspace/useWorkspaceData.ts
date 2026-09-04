import { useStore } from "zustand";
import { useEffect } from "preact/hooks";
import { workspaceApi } from "../../../api/workspaceApi";
import type {
  WorkspaceSnapshot,
  WorkspaceStoreActions,
} from "../../../models/workspace";
import { createWorkspaceStore } from "../../stores/workspace/workspaceStore";

interface WorkspaceFeed
  extends WorkspaceSnapshot, Pick<WorkspaceStoreActions, "seedChat"> {}

// One feed for the whole app. The concrete socket is wired here rather than
// inside the store so the store stays free of the api layer and testable.
const workspaceStore = createWorkspaceStore(workspaceApi.subscribe);

// Defined once with the store, so a caller may list it as an effect or callback
// dependency without churning.
const seedChat = workspaceStore.getState().seedChat;

/** The chats and projects the server is pushing. */
export function useWorkspaceData(enabled: boolean): WorkspaceFeed {
  const snapshot = useStore(workspaceStore, (state) => state.snapshot);

  useEffect(() => {
    workspaceStore.getState().setConnected(enabled);
    return () => {
      workspaceStore.getState().setConnected(false);
    };
  }, [enabled]);

  return { ...snapshot, seedChat };
}

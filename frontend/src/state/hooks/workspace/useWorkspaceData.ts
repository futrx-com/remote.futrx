import { useStore } from "zustand";
import { useEffect } from "preact/hooks";
import { workspaceApi } from "../../../api/workspaceApi";
import type { WorkspaceSnapshot } from "../../../models/workspace";
import { createWorkspaceStore } from "../../stores/workspace/workspaceStore";

// One feed for the whole app. The concrete socket is wired here rather than
// inside the store so the store stays free of the api layer and testable.
const workspaceStore = createWorkspaceStore(workspaceApi.subscribe);

/** The chats and projects the server is pushing. */
export function useWorkspaceData(enabled: boolean): WorkspaceSnapshot {
  const snapshot = useStore(workspaceStore, (state) => state.snapshot);

  useEffect(() => {
    workspaceStore.getState().setConnected(enabled);
    return () => {
      workspaceStore.getState().setConnected(false);
    };
  }, [enabled]);

  return snapshot;
}

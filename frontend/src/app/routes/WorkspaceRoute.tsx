import { WorkspaceProvider } from "../../state/context/WorkspaceContext";
import { useExtensions } from "../extensions/useExtensions";
import { WorkspaceContainer } from "../containers/WorkspaceContainer";
import { WorkspaceSearchContext } from "../../state/context/WorkspaceSearchContext";
import { workspaceSearchSurfaces } from "../workspaceSearch";

export function WorkspaceRoute({ enabled }: { enabled: boolean }) {
  useExtensions(enabled);

  return (
    <WorkspaceProvider enabled={enabled}>
      <WorkspaceSearchContext.Provider value={workspaceSearchSurfaces}>
        <WorkspaceContainer />
      </WorkspaceSearchContext.Provider>
    </WorkspaceProvider>
  );
}

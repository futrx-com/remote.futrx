import { WorkspaceProvider } from "../../state/context/WorkspaceContext";
import { useExtensions } from "../extensions/useExtensions";
import { WorkspaceContainer } from "../containers/WorkspaceContainer";

export function WorkspaceRoute({ enabled }: { enabled: boolean }) {
  useExtensions(enabled);

  return (
    <WorkspaceProvider enabled={enabled}>
      <WorkspaceContainer />
    </WorkspaceProvider>
  );
}

import { AppProviders } from "./AppProviders";
import { AuthGate } from "./containers/AuthGate";
import { useViewportHeightSync } from "../state/hooks/platform/useViewportHeightSync";
import { useFrontendBuildSync } from "../state/hooks/server/useFrontendBuildSync";

export function App() {
  useViewportHeightSync();
  useFrontendBuildSync();

  return (
    <AppProviders>
      <AuthGate />
    </AppProviders>
  );
}

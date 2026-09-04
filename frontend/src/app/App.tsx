import { AppProviders } from "./AppProviders";
import { AuthGate } from "./containers/AuthGate";
import { useViewportHeightSync } from "../state/hooks/platform/useViewportHeightSync";

export function App() {
  useViewportHeightSync();

  return (
    <AppProviders>
      <AuthGate />
    </AppProviders>
  );
}

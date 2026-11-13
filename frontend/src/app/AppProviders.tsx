import { PermissionLookupContext } from "../state/context/PermissionLookupContext";
import { permissionLookup } from "./permissions";
import type { ComponentChildren } from "preact";
import { AuthProvider } from "../state/context/AuthContext";
import { ConfirmProvider } from "./containers/ConfirmProvider";
import { UserSettingsProvider } from "../state/context/UserSettingsContext";

export function AppProviders({ children }: { children: ComponentChildren }) {
  return (
    <AuthProvider>
      <UserSettingsProvider>
        <ConfirmProvider>
          <PermissionLookupContext.Provider value={permissionLookup}>
            {children}
          </PermissionLookupContext.Provider>
        </ConfirmProvider>
      </UserSettingsProvider>
    </AuthProvider>
  );
}

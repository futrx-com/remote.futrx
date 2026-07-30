import type { ComponentChildren } from "preact";
import { createContext } from "preact";
import { useContext } from "preact/hooks";
import { useAuth, type AuthState } from "../hooks/auth/useAuth";
import { useClaudeAuth, type ClaudeAuthState } from "../hooks/auth/useClaudeAuth";
import { useCodexAuth, type CodexAuthState } from "../hooks/auth/useCodexAuth";
import { useCustomAuth, type CustomAuthState } from "../hooks/auth/useCustomAuth";
import { useKimiAuth, type KimiAuthState } from "../hooks/auth/useKimiAuth";

interface AuthContextValue {
  auth: AuthState;
  claudeAuth: ClaudeAuthState;
  codexAuth: CodexAuthState;
  kimiAuth: KimiAuthState;
  customAuth: CustomAuthState;
  appAuthOk: boolean;
  providerAuthChecked: boolean;
  providerAuthenticated: boolean;
  gateOpen: boolean;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ComponentChildren }) {
  const auth = useAuth();
  // A valid local-admin or invited-user session may proceed to provider setup.
  const appAuthOk = auth.authenticated && (auth.isRegistered || auth.isAdmin);
  const providerAuthEnabled = appAuthOk && auth.localAdminConfigured;
  const claudeAuth = useClaudeAuth(providerAuthEnabled);
  const codexAuth = useCodexAuth(providerAuthEnabled);
  const kimiAuth = useKimiAuth(providerAuthEnabled);
  const customAuth = useCustomAuth(providerAuthEnabled);
  const providerAuthChecked =
    claudeAuth.checked && codexAuth.checked && kimiAuth.checked && customAuth.checked;
  const providerAuthenticated =
    claudeAuth.authenticated ||
    codexAuth.authenticated ||
    kimiAuth.authenticated ||
    customAuth.authenticated;
  const gateOpen = providerAuthEnabled && providerAuthChecked && providerAuthenticated;

  return (
    <AuthContext.Provider
      value={{
        auth,
        claudeAuth,
        codexAuth,
        kimiAuth,
        customAuth,
        appAuthOk,
        providerAuthChecked,
        providerAuthenticated,
        gateOpen,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuthContext(): AuthContextValue {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuthContext must be used inside AuthProvider");
  return value;
}

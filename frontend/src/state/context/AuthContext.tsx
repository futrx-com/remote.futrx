import type { ComponentChildren } from "preact";
import { createContext } from "preact";
import { useContext, useEffect, useMemo, useRef } from "preact/hooks";
import { agentCapabilityCatalogStore } from "../stores/agents/agentCapabilityCatalogStore";
import { agentAuthRegistryService } from "../../services/auth/agentAuthRegistryService.ts";
import { useAgentAuthRegistry, type AgentAuthRegistryState } from "../hooks/auth/useAgentAuthRegistry";
import { useAuth, type AuthState } from "../hooks/auth/useAuth";

interface AuthContextValue {
  auth: AuthState;
  agentAuth: AgentAuthRegistryState;
  appAuthOk: boolean;
  providerAuthChecked: boolean;
  providerAuthenticated: boolean;
  gateOpen: boolean;
}

const AuthContext = createContext<AuthContextValue | null>(null);

interface ProviderAuthMarker {
  userId: string;
  revision: string;
}

export function AuthProvider({ children }: { children: ComponentChildren }) {
  ////////////////
  // Local State
  ////////////////
  const auth = useAuth();
  // A valid local-admin or invited-user session may proceed to provider setup.
  const appAuthOk = auth.authenticated && (auth.isRegistered || auth.isAdmin);
  const providerAuthEnabled = appAuthOk && auth.localAdminConfigured;
  const agentAuth = useAgentAuthRegistry(providerAuthEnabled);
  const providerAuthChecked = agentAuth.checked;
  const providerAuthenticated = agentAuth.gateReady;
  const gateOpen = providerAuthEnabled && providerAuthChecked && providerAuthenticated;
  const revision = agentAuthRegistryService.revision(agentAuth.providers);

  ////////////////
  // Refs
  ////////////////
  const previousProviderAuth = useRef<ProviderAuthMarker | null>(null);

  ////////////////
  // Effects
  ////////////////
  useEffect(() => {
    const userId = auth.email || auth.adminEmail;
    if (!providerAuthChecked || !userId) return;

    const current: ProviderAuthMarker = {
      userId: userId.trim().toLowerCase(),
      revision,
    };
    const previous = previousProviderAuth.current;
    previousProviderAuth.current = current;
    if (!previous || previous.userId !== current.userId || previous.revision === current.revision) {
      return;
    }

    // Provider identity and entitlements affect live model discovery. Request
    // a refresh for every capability scope mounted in this browser.
    agentCapabilityCatalogStore.getState().invalidateUser(current.userId);
  }, [auth.email, auth.adminEmail, providerAuthChecked, revision]);

  ////////////////
  // Context Value
  ////////////////
  // preact force-renders every subscriber whenever this value fails a `!=`
  // check, and ten call sites read this context — the sidebar and the composer
  // among them. A fresh literal here repainted all of them on every agent-auth
  // websocket push, however unrelated.
  const value = useMemo<AuthContextValue>(() => ({
    auth,
    agentAuth,
    appAuthOk,
    providerAuthChecked,
    providerAuthenticated,
    gateOpen,
  }), [auth, agentAuth, appAuthOk, providerAuthChecked, providerAuthenticated, gateOpen]);

  return (
    <AuthContext.Provider value={value}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuthContext(): AuthContextValue {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuthContext must be used inside AuthProvider");
  return value;
}

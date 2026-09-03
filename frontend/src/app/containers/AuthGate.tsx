import type { ComponentType } from "preact";
import { useEffect, useState } from "preact/hooks";
import { LoginScreen } from "../../ui/auth/LoginScreen";
import { AdminSetupWaiting } from "../../ui/auth/AdminSetupWaiting";
import { ProviderAuthWaiting } from "../../ui/auth/ProviderAuthWaiting";
import { ProviderLoginScreen } from "../../ui/auth/ProviderLoginScreen";
import { LoadingScreen } from "../../ui/primitives/LoadingScreen";
import { WorkspaceSkeleton } from "../../ui/layout/WorkspaceSkeleton";
import { useAuthContext } from "../../state/context/AuthContext";
import { useAdminSetupPolling } from "../../state/hooks/auth/useAdminSetupPolling";
import {
  expectsWorkspace,
  rememberWorkspaceBoot,
} from "./workspaceBootHint";

type WorkspaceRouteComponent = ComponentType<{ enabled: boolean }>;

export function AuthGate() {
  const {
    auth,
    appAuthOk,
    providerAuthChecked,
    providerAuthenticated,
    gateOpen,
  } = useAuthContext();
  const [WorkspaceRoute, setWorkspaceRoute] = useState<WorkspaceRouteComponent | null>(null);
  const currentUserCanSetupLocalAdmin =
    auth.isAdmin && auth.email.toLowerCase() === auth.adminEmail.toLowerCase();
  const waitingForAdminSetup =
    !auth.loading && auth.claimed && appAuthOk &&
    !auth.localAdminConfigured && !currentUserCanSetupLocalAdmin;

  useAdminSetupPolling(auth.refresh, waitingForAdminSetup);
  // Read once per mount: the hint describes what this boot should paint, and
  // must not flip mid-boot when the current outcome is recorded below.
  const [bootsIntoWorkspace] = useState(expectsWorkspace);

  useEffect(() => {
    if (auth.loading) return;
    rememberWorkspaceBoot(gateOpen);
  }, [auth.loading, gateOpen]);

  useEffect(() => {
    if (!gateOpen || WorkspaceRoute) return;
    let cancelled = false;
    import("../routes/WorkspaceRoute").then((module) => {
      if (!cancelled) setWorkspaceRoute(() => module.WorkspaceRoute);
    });
    return () => {
      cancelled = true;
    };
  }, [gateOpen, WorkspaceRoute]);

  // A returning user gets the workspace outline straight away; anyone whose
  // last visit stopped at a login screen gets the neutral boot screen instead.
  if (auth.loading) return bootsIntoWorkspace ? <WorkspaceSkeleton /> : <LoadingScreen />;
  if (!auth.claimed) {
    return (
      <LoginScreen
        mode="claim"
        adminEmail=""
        localAdminConfigured={false}
        googleOAuthEnabled={false}
        onSuccess={auth.refresh}
      />
    );
  }
  if (!appAuthOk) {
    return (
      <LoginScreen
        mode="login"
        adminEmail={auth.adminEmail}
        localAdminConfigured={auth.localAdminConfigured}
        googleOAuthEnabled={auth.googleOAuthEnabled}
        onSuccess={auth.refresh}
      />
    );
  }
  if (!auth.localAdminConfigured) {
    if (currentUserCanSetupLocalAdmin) {
      return (
        <LoginScreen
          mode="legacy-setup"
          adminEmail={auth.email}
          localAdminConfigured={false}
          googleOAuthEnabled={auth.googleOAuthEnabled}
          onSuccess={auth.refresh}
        />
      );
    }
    return <AdminSetupWaiting adminEmail={auth.adminEmail} />;
  }
  // Past this point the destination is the workspace, so the outline stands in
  // for it rather than a spinner.
  if (!providerAuthChecked) return <WorkspaceSkeleton />;
  if (!providerAuthenticated) {
    // Members wait while an administrator completes any module-declared
    // access path. The catalog may expose managed or no-auth gate providers.
    if (auth.isAdmin) {
      return <ProviderLoginScreen />;
    }
    return <ProviderAuthWaiting adminEmail={auth.adminEmail} />;
  }

  if (!WorkspaceRoute) return <WorkspaceSkeleton />;
  return <WorkspaceRoute enabled={gateOpen} />;
}

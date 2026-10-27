import type { SecuritySettingsController } from "../../state/hooks/auth/useSecuritySettings";
import { SecurityAlertBanner } from "./security/SecurityAlertBanner";
import { SecurityPreferenceToggle } from "./security/SecurityPreferenceToggle";
import { SessionHistoryList } from "./security/SessionHistoryList";
import { TwoFactorSettings } from "./security/TwoFactorSettings";
import { Loader } from "../primitives/icons";

export function SecuritySettings({ controller }: { controller: SecuritySettingsController }) {
  const { settings, loading, error } = controller;

  if (loading && !settings) {
    return (
      <div class="flex items-center gap-2 text-[13px] text-ink-300">
        <Loader class="w-4 h-4 animate-spin" /> Loading security settings…
      </div>
    );
  }

  return (
    <div class="space-y-4">
      {error && <div class="text-xs text-accent-red">{error}</div>}
      {settings?.securityAlert && (
        <SecurityAlertBanner
          alert={settings.securityAlert}
          onAck={controller.acknowledgeAlert}
        />
      )}
      <TwoFactorSettings controller={controller} />
      {settings?.twoFactorEnabled && (
        <SecurityPreferenceToggle
          title="Alert on depleted recovery codes"
          description="Show an alert when you are running low on recovery codes to avoid getting locked out."
          checked={settings.recoveryCodeAlertEnabled ?? false}
          onChange={controller.setRecoveryCodeAlertEnabled}
        />
      )}
      <SecurityPreferenceToggle
        title="Single active session"
        description="Signing in on a new device immediately signs you out everywhere else."
        checked={settings?.singleSessionEnabled ?? false}
        onChange={controller.setSingleSessionEnabled}
      />
      <SecurityPreferenceToggle
        title="Sign-in history"
        description="Keep a record of the devices and locations that have signed in to this account."
        checked={settings?.historyEnabled ?? false}
        onChange={controller.setHistoryEnabled}
      />
      {settings?.historyEnabled && <SessionHistoryList sessions={settings.sessions} />}
    </div>
  );
}

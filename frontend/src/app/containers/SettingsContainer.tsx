import { useCallback, useState } from "preact/hooks";
import {
  SettingsPage,
  type SettingsTab,
} from "../../ui/settings/SettingsPage";
import { useAuthContext } from "../../state/context/AuthContext";
import { useUserSettingsContext } from "../../state/context/UserSettingsContext";
import { useUserDirectory } from "../../state/hooks/users/useUserDirectory";
import { useServerInfo } from "../../state/hooks/server/useServerInfo";
import { useSelfUpdate } from "../../state/hooks/server/useSelfUpdate";
import { usePushNotifications } from "../../state/hooks/push/usePushNotifications";
import { useUsageDashboard } from "../../state/hooks/usage/useUsageDashboard";
import { usageApi } from "../../api/usageApi";

export function SettingsContainer({
  onBack,
  onHamburger,
}: {
  onBack: () => void;
  onHamburger: () => void;
}) {
  const { auth } = useAuthContext();
  const userSettings = useUserSettingsContext();
  const userDirectory = useUserDirectory(auth.isAdmin);
  const [activeTab, setActiveTab] = useState<SettingsTab>("appearance");
  const serverInfo = useServerInfo(activeTab === "info");
  const selfUpdate = useSelfUpdate(activeTab === "updates" && auth.isAdmin);
  const usageDashboard = useUsageDashboard(activeTab === "usage");
  const [usageRebuilding, setUsageRebuilding] = useState(false);
  const [usageRebuildMessage, setUsageRebuildMessage] = useState<string | null>(null);

  const rebuildUsage = useCallback(async () => {
    setUsageRebuilding(true);
    setUsageRebuildMessage(null);
    try {
      const result = await usageApi.rebuild();
      setUsageRebuildMessage(
        `Rebuilt ${result.records} record${result.records === 1 ? "" : "s"} from ${result.chats} chat${
          result.chats === 1 ? "" : "s"
        }.`
      );
      await usageDashboard.refresh();
    } catch (cause) {
      setUsageRebuildMessage(`Rebuild failed: ${(cause as Error).message}`);
    } finally {
      setUsageRebuilding(false);
    }
  }, [usageDashboard]);
  const push = usePushNotifications(
    activeTab === "notifications",
    auth.email || auth.adminEmail
  );

  return (
    <SettingsPage
      activeTab={activeTab}
      currentEmail={auth.email}
      isAdmin={auth.isAdmin}
      googleOAuthEnabled={auth.googleOAuthEnabled}
      serverInfo={serverInfo.info}
      serverInfoLoading={serverInfo.loading}
      serverInfoRefreshing={serverInfo.refreshing}
      serverInfoError={serverInfo.error}
      selfUpdate={selfUpdate.status}
      selfUpdateLoading={selfUpdate.loading}
      selfUpdateChecking={selfUpdate.checking}
      selfUpdateApplying={selfUpdate.applying}
      selfUpdateRestarting={selfUpdate.restarting}
      selfUpdateError={selfUpdate.error}
      userDirectory={userDirectory}
      usageDashboard={usageDashboard}
      usageRebuilding={usageRebuilding}
      usageRebuildMessage={usageRebuildMessage}
      onRebuildUsage={rebuildUsage}
      appearanceTheme={userSettings.settings.appearance.theme}
      appearanceLoading={userSettings.loading}
      appearanceSaving={userSettings.saving}
      appearanceError={userSettings.error}
      push={push}
      onBack={onBack}
      onHamburger={onHamburger}
      onTabChange={setActiveTab}
      onRefreshServerInfo={serverInfo.refresh}
      onCheckForUpdates={selfUpdate.check}
      onApplyUpdate={selfUpdate.apply}
      onAppearanceThemeChange={(theme) => void userSettings.setTheme(theme)}
    />
  );
}

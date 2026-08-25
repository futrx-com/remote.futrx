import { useState } from "preact/hooks";
import {
  SettingsPage,
  type SettingsTab,
} from "../../ui/settings/SettingsPage";
import { useAuthContext } from "../../state/context/AuthContext";
import { useUserSettingsContext } from "../../state/context/UserSettingsContext";
import { useUserDirectory } from "../../state/hooks/users/useUserDirectory";
import { useServerInfo } from "../../state/hooks/server/useServerInfo";
import { useSelfUpdate } from "../../state/hooks/server/useSelfUpdate";
import { useGlobalApplications } from "../../state/hooks/applications/useApplications";
import { usePushNotifications } from "../../state/hooks/push/usePushNotifications";

export function SettingsContainer({
  onBack,
  onHamburger,
}: {
  onBack: () => void;
  onHamburger: () => void;
}) {
  const { auth, codexAuth, kimiAuth } = useAuthContext();
  const userSettings = useUserSettingsContext();
  const userDirectory = useUserDirectory(auth.isAdmin);
  const [activeTab, setActiveTab] = useState<SettingsTab>("appearance");
  const serverInfo = useServerInfo(activeTab === "info");
  const selfUpdate = useSelfUpdate(activeTab === "updates" && auth.isAdmin);
  const applications = useGlobalApplications(
    activeTab === "applications" && auth.isAdmin
  );
  const push = usePushNotifications(activeTab === "notifications");

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
      appearanceTheme={userSettings.settings.appearance.theme}
      appearanceLoading={userSettings.loading}
      appearanceSaving={userSettings.saving}
      appearanceError={userSettings.error}
      push={push}
      codexAuthenticated={codexAuth.authenticated}
      codexUsesApiKey={codexAuth.usesApiKey}
      codexDeviceLogin={codexAuth.deviceLogin}
      codexLoading={codexAuth.loading}
      codexStarting={codexAuth.starting}
      codexError={codexAuth.error}
      onBack={onBack}
      onHamburger={onHamburger}
      onTabChange={setActiveTab}
      onRefreshServerInfo={serverInfo.refresh}
      onCheckForUpdates={selfUpdate.check}
      onApplyUpdate={selfUpdate.apply}
      onAppearanceThemeChange={(theme) => void userSettings.setTheme(theme)}
      onStartCodexDeviceLogin={codexAuth.startDeviceLogin}
      kimiAuthenticated={kimiAuth.authenticated}
      kimiDeviceLogin={kimiAuth.deviceLogin}
      kimiLoading={kimiAuth.loading}
      kimiStarting={kimiAuth.starting}
      kimiError={kimiAuth.error}
      onStartKimiDeviceLogin={kimiAuth.startDeviceLogin}
      applications={applications}
    />
  );
}

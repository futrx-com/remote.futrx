import { useState } from "preact/hooks";
import {
  SettingsPage,
  type SettingsTab,
} from "../../ui/settings/SettingsPage";
import { useAuthContext } from "../../state/context/AuthContext";
import { useUserSettingsContext } from "../../state/context/UserSettingsContext";
import { useUserDirectory } from "../../state/hooks/users/useUserDirectory";
import { useGlobalSkills } from "../../state/hooks/settings/useGlobalSkills";
import { useWorkspaceContext } from "../../state/context/WorkspaceContext";
import { useServerInfo } from "../../state/hooks/server/useServerInfo";
import { useSelfUpdate } from "../../state/hooks/server/useSelfUpdate";

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
  const { projects } = useWorkspaceContext();
  const [activeTab, setActiveTab] = useState<SettingsTab>("appearance");
  const globalSkills = useGlobalSkills(activeTab === "skills" && auth.isAdmin);
  const serverInfo = useServerInfo(activeTab === "info");
  const selfUpdate = useSelfUpdate(activeTab === "updates" && auth.isAdmin);

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
      globalSkills={globalSkills}
      projects={projects}
      appearanceTheme={userSettings.settings.appearance.theme}
      appearanceLoading={userSettings.loading}
      appearanceSaving={userSettings.saving}
      appearanceError={userSettings.error}
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
    />
  );
}

import type { AppearanceTheme } from "../../models/settings";
import type { UserDirectory } from "../../state/hooks/users/useUserDirectory";
import type { ServerInfo } from "../../models/serverInfo";
import type { SelfUpdateStatus } from "../../models/selfUpdate";
import type { SecuritySettingsController } from "../../state/hooks/auth/useSecuritySettings";
import type { ComponentType } from "preact";
import type { PushNotifications } from "../../state/hooks/push/usePushNotifications";
import {
  Activity,
  Bell,
  Bot,
  ChevronLeft,
  Download,
  Info,
  Menu,
  Monitor,
  ShieldCheck,
  Users,
} from "../primitives/icons";
import { AppearanceSettings } from "./AppearanceSettings";
import { NotificationSettings } from "./NotificationSettings";
import { AgentAuthSettingsList } from "./AgentAuthSettings";
import { GoogleOAuthSettings } from "./GoogleOAuthSettings";
import { SecuritySettings } from "./SecuritySettings";
import { ServerInfoSettings } from "./ServerInfoSettings";
import { UpdatesSettings } from "./UpdatesSettings";
import { UsageSettings } from "./UsageSettings";
import { UsersPanel } from "../account/UsersPanel";
import type { UsageDashboard } from "../../state/hooks/usage/useUsageDashboard";

export type SettingsTab =
  | "appearance"
  | "notifications"
  | "agents"
  | "users"
  | "security"
  | "updates"
  | "info"
  | "usage";

const tabs: Array<{
  id: SettingsTab;
  label: string;
  description: string;
  Icon: ComponentType<{ class?: string }>;
}> = [
  {
    id: "appearance",
    label: "Appearance",
    description: "Choose how Remote looks on this device.",
    Icon: Monitor,
  },
  {
    id: "notifications",
    label: "Notifications",
    description: "Get alerted when an agent needs you.",
    Icon: Bell,
  },
  {
    id: "agents",
    label: "Agents",
    description: "Configure coding-agent access and authentication.",
    Icon: Bot,
  },
  {
    id: "usage",
    label: "Usage",
    description: "Track tokens and estimated cost per project, user, provider, and model.",
    Icon: Activity,
  },
  {
    id: "users",
    label: "Users",
    description: "Control who can access this server.",
    Icon: Users,
  },
  {
    id: "security",
    label: "Security",
    description: "Manage two-factor authentication, sessions, and sign-in history.",
    Icon: ShieldCheck,
  },
  {
    id: "updates",
    label: "Updates",
    description: "Check for new releases and install them.",
    Icon: Download,
  },
  {
    id: "info",
    label: "Info",
    description: "View details about the main parent server.",
    Icon: Info,
  },
];

export function SettingsPage({
  activeTab,
  currentEmail,
  isAdmin,
  googleOAuthEnabled,
  serverInfo,
  serverInfoLoading,
  serverInfoRefreshing,
  serverInfoError,
  selfUpdate,
  selfUpdateLoading,
  selfUpdateChecking,
  selfUpdateApplying,
  selfUpdateRestarting,
  selfUpdateError,
  userDirectory,
  usageDashboard,
  usageRebuilding,
  usageRebuildMessage,
  onRebuildUsage,
  appearanceTheme,
  appearanceLoading,
  appearanceSaving,
  appearanceError,
  push,
  security,
  onBack,
  onHamburger,
  onTabChange,
  onRefreshServerInfo,
  onCheckForUpdates,
  onApplyUpdate,
  onAppearanceThemeChange,
}: {
  activeTab: SettingsTab;
  currentEmail: string;
  isAdmin: boolean;
  googleOAuthEnabled: boolean;
  serverInfo: ServerInfo | null;
  serverInfoLoading: boolean;
  serverInfoRefreshing: boolean;
  serverInfoError: string | null;
  selfUpdate: SelfUpdateStatus | null;
  selfUpdateLoading: boolean;
  selfUpdateChecking: boolean;
  selfUpdateApplying: boolean;
  selfUpdateRestarting: boolean;
  selfUpdateError: string | null;
  userDirectory: UserDirectory;
  usageDashboard: UsageDashboard;
  usageRebuilding: boolean;
  usageRebuildMessage: string | null;
  onRebuildUsage: () => Promise<void>;
  appearanceTheme: AppearanceTheme;
  appearanceLoading: boolean;
  appearanceSaving: boolean;
  appearanceError: string | null;
  push: PushNotifications;
  security: SecuritySettingsController;
  onBack: () => void;
  onHamburger: () => void;
  onTabChange: (tab: SettingsTab) => void;
  onRefreshServerInfo: () => Promise<void>;
  onCheckForUpdates: () => Promise<void>;
  onApplyUpdate: (tag?: string) => Promise<void>;
  onAppearanceThemeChange: (theme: AppearanceTheme) => void;
}) {
  const activeTabDetails = tabs.find((tab) => tab.id === activeTab) ?? tabs[0];

  return (
    <div class="flex-1 flex flex-col min-h-0 overflow-hidden">
      <header class="codex-header top-chrome z-20 flex-none border-b border-line px-3 pb-2 flex items-center gap-2 min-h-[52px]">
        <button
          type="button"
          onClick={onHamburger}
          class="md:hidden h-10 w-10 text-ink-100 rounded-md hover:bg-tint-strong grid place-items-center"
          aria-label="Toggle sidebar"
        >
          <Menu class="w-5 h-5" />
        </button>
        <button
          type="button"
          onClick={onBack}
          class="hidden md:inline-flex items-center gap-1.5 h-10 px-2 text-ink-200 hover:text-ink-50
                 hover:bg-tint-strong rounded-md text-sm"
        >
          <ChevronLeft class="w-4 h-4" /> Chats
        </button>
        <div class="flex-1 min-w-0">
          <div class="text-[11px] text-ink-300">Preferences</div>
          <div class="text-[15px] font-semibold text-ink-50 truncate">Settings</div>
        </div>
      </header>

      <div class="flex-1 min-h-0 flex flex-col md:flex-row overflow-hidden">
        <SettingsNavigation
          activeTab={activeTab}
          onTabChange={onTabChange}
          className="theme-submenu-surface hidden md:flex w-56 flex-none border-r border-line bg-inset p-3"
        />
        <SettingsNavigation
          activeTab={activeTab}
          onTabChange={onTabChange}
          mobile
          className="theme-submenu-surface md:hidden flex-none border-b border-line bg-inset px-3 py-2 overflow-x-auto no-scrollbar"
        />

        <main
          id="settings-active-panel"
          role="tabpanel"
          class="flex-1 min-w-0 overflow-y-auto touch-scroll"
        >
          <div class="w-full px-4 py-5 md:px-6 md:py-7">
            <header class="mb-5">
              <h1 class="text-xl font-semibold text-ink-50">{activeTabDetails.label}</h1>
              <p class="mt-1 text-[13px] leading-relaxed text-ink-300">
                {activeTabDetails.description}
              </p>
            </header>

            {activeTab === "appearance" && (
              <AppearanceSettings
                theme={appearanceTheme}
                loading={appearanceLoading}
                saving={appearanceSaving}
                error={appearanceError}
                onThemeChange={onAppearanceThemeChange}
              />
            )}

            {activeTab === "notifications" && <NotificationSettings push={push} />}

            {activeTab === "agents" && (
              isAdmin ? (
                <div class="rounded-card border border-line bg-surface overflow-hidden">
                  <div class="px-4 py-3 border-b border-line">
                    <div class="text-[14.5px] font-semibold text-ink-50">Agent authentication</div>
                    <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">
                      Configure each agent using its declared host-managed, project-external, or no-auth flow.
                    </div>
                  </div>
                  <div class="p-3 space-y-3">
                    <AgentAuthSettingsList />
                  </div>
                </div>
              ) : (
                <SettingsNotice>
                  Agent authentication is managed by server administrators.
                </SettingsNotice>
              )
            )}

            {activeTab === "usage" && (
              <UsageSettings
                dashboard={usageDashboard}
                isAdmin={isAdmin}
                onRebuild={onRebuildUsage}
                rebuilding={usageRebuilding}
                rebuildMessage={usageRebuildMessage}
              />
            )}

            {activeTab === "users" && (
              <div class="space-y-4">
                {isAdmin && <GoogleOAuthSettings />}
                <UsersPanel
                  currentEmail={currentEmail}
                  isAdmin={isAdmin}
                  users={userDirectory.users}
                  loading={userDirectory.loading}
                  error={userDirectory.error}
                  canAddUsers={googleOAuthEnabled}
                  onAdd={userDirectory.add}
                  onRemove={userDirectory.remove}
                  onSetRole={userDirectory.setRole}
                />
              </div>
            )}

            {activeTab === "security" && <SecuritySettings controller={security} />}

            {activeTab === "updates" &&
              (isAdmin ? (
                <UpdatesSettings
                  status={selfUpdate}
                  loading={selfUpdateLoading}
                  checking={selfUpdateChecking}
                  applying={selfUpdateApplying}
                  restarting={selfUpdateRestarting}
                  error={selfUpdateError}
                  onCheck={onCheckForUpdates}
                  onApply={onApplyUpdate}
                />
              ) : (
                <SettingsNotice>
                  Application updates are managed by server administrators.
                </SettingsNotice>
              ))}

            {activeTab === "info" && (
              <ServerInfoSettings
                currentEmail={currentEmail}
                isAdmin={isAdmin}
                info={serverInfo}
                loading={serverInfoLoading}
                refreshing={serverInfoRefreshing}
                error={serverInfoError}
                onRefresh={onRefreshServerInfo}
              />
            )}
          </div>
        </main>
      </div>
    </div>
  );
}

function SettingsNavigation({
  activeTab,
  onTabChange,
  mobile = false,
  className,
}: {
  activeTab: SettingsTab;
  onTabChange: (tab: SettingsTab) => void;
  mobile?: boolean;
  className: string;
}) {
  return (
    <aside class={className} aria-label="Settings sections">
      <nav
        class={mobile ? "flex gap-1 min-w-max" : "w-full space-y-1"}
        role="tablist"
        aria-orientation={mobile ? "horizontal" : "vertical"}
      >
        {tabs.map(({ id, label, Icon }) => {
          const active = id === activeTab;
          return (
            <button
              key={id}
              type="button"
              role="tab"
              aria-selected={active}
              aria-controls="settings-active-panel"
              tabIndex={active ? 0 : -1}
              onClick={() => onTabChange(id)}
              class={`${mobile ? "h-9 px-3" : "w-full h-10 px-3"} rounded-md inline-flex items-center gap-2.5 border text-[13px] font-medium transition-colors ${
                active
                  ? "border-line bg-tint-strong text-ink-50"
                  : "border-transparent text-ink-300 hover:text-ink-100 hover:bg-tint"
              }`}
            >
              <Icon class={`w-4 h-4 flex-none ${active ? "text-accent-blue" : "text-ink-400"}`} />
              <span>{label}</span>
            </button>
          );
        })}
      </nav>
    </aside>
  );
}

function SettingsNotice({ children }: { children: string }) {
  return (
    <section class="rounded-card border border-line bg-surface p-4 text-[13px] leading-relaxed text-ink-300">
      {children}
    </section>
  );
}

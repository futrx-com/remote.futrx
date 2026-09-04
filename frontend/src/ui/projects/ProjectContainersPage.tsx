import type { ComponentChildren, ComponentType } from "preact";
import { useCallback, useState } from "preact/hooks";
import type {
  AccessRecord,
  ProjectContainerRecord,
  SecretsRecord,
} from "../../models/project";
import { Empty } from "./project-containers/ProjectContainerPrimitives";
import { ProjectActions } from "./project-containers/ProjectActions";
import {
  ContainerStateBadge,
  ProjectInfoSection,
} from "./project-containers/ProjectInfoSection";
import {
  ProjectSecretsSection,
  type SecretDraft,
} from "./project-containers/ProjectSecretsSection";
import { ProjectSharingSection } from "./project-containers/ProjectSharingSection";
import { ProjectResourceLimits } from "./project-containers/ProjectResourceLimits";
import { ProjectUsageLine } from "./project-containers/ProjectUsageLine";
import { formatRelativeTime as fmtRelative } from "./project-containers/projectContainerFormat";
import type { ContainerLimits, ProjectContainerInfo, ProjectMeta } from "../../models/project";
import type { UsageSummary } from "../../models/usage";
import { ChevronLeft, Info, Key, Loader, Menu, RotateCcw, Settings, Users } from "../primitives/icons";
import { useConfirm } from "../../state/context/ConfirmContext";

export type ProjectSettingsTab = "info" | "settings" | "secrets" | "sharing";

const tabs: Array<{
  id: ProjectSettingsTab;
  label: string;
  description: string;
  Icon: ComponentType<{ class?: string }>;
}> = [
  {
    id: "info",
    label: "Info",
    description: "Inspect the container, operating system, resources, network, and agent tooling.",
    Icon: Info,
  },
  {
    id: "settings",
    label: "Settings",
    description: "Manage this project's container lifecycle and destructive actions.",
    Icon: Settings,
  },
  {
    id: "secrets",
    label: "Secrets",
    description: "Configure environment secrets passed to agents in this project.",
    Icon: Key,
  },
  {
    id: "sharing",
    label: "Sharing",
    description: "Control which registered users can access this project.",
    Icon: Users,
  },
];

export function ProjectContainersPage({
  activeTab,
  project,
  infoRecord,
  secretsRecord,
  accessRecord,
  refreshing,
  isAdmin,
  serverMemoryTotalBytes,
  serverMemoryLoading,
  usageSummary,
  usageLoading,
  usageError,
  onRefresh,
  onBack,
  onHamburger,
  onTabChange,
  onSaveSecret,
  onDeleteSecret,
  onAddMember,
  onRemoveMember,
  onRepairNetwork,
  onSetResourceLimits,
  onStartProject,
  onStopProject,
  onRestartProject,
  onDeleteProject,
}: {
  activeTab: ProjectSettingsTab;
  project: ProjectMeta | null;
  infoRecord: ProjectContainerRecord;
  secretsRecord: SecretsRecord;
  accessRecord: AccessRecord;
  refreshing: boolean;
  isAdmin: boolean;
  serverMemoryTotalBytes?: number;
  serverMemoryLoading: boolean;
  usageSummary: UsageSummary | null;
  usageLoading: boolean;
  usageError: string | null;
  onRefresh: () => void;
  onBack: () => void;
  onHamburger: () => void;
  onTabChange: (tab: ProjectSettingsTab) => void;
  onSaveSecret: (key: string, value: string) => Promise<void>;
  onDeleteSecret: (key: string) => Promise<void>;
  onAddMember: (email: string) => Promise<void>;
  onRemoveMember: (email: string) => Promise<void>;
  onRepairNetwork: () => Promise<void>;
  onSetResourceLimits: (limits: ContainerLimits) => Promise<void>;
  onStartProject: () => Promise<void>;
  onStopProject: () => Promise<void>;
  onRestartProject: () => Promise<void>;
  onDeleteProject: () => Promise<void>;
}) {
  const confirm = useConfirm();
  const activeTabDetails = tabs.find((tab) => tab.id === activeTab) ?? tabs[0];

  // Draft state for the "Add new secret" form — owned here so it survives tab
  // switches. SecretEditor is a conditionally-rendered subtree that would
  // otherwise be unmounted (and lose its local state) on every navigation.
  const [secretDraft, setSecretDraft] = useState<SecretDraft>({ key: "", value: "" });

  const hasDraft = secretDraft.key.trim() !== "" || secretDraft.value !== "";

  // Guard tab navigation: if there is a non-empty draft and the user is leaving
  // the Secrets tab, ask for confirmation before silently discarding it.
  const handleTabChange = useCallback(
    async (next: ProjectSettingsTab) => {
      if (next === activeTab) return;
      if (activeTab === "secrets" && hasDraft) {
        const ok = await confirm({
          title: "Discard unsaved secret?",
          message: `You have an unsaved draft for "${secretDraft.key || "(no key)"}". Switching tabs will keep the draft — it will be here when you return.`,
          confirmLabel: "Switch anyway",
          cancelLabel: "Stay on Secrets",
          tone: "neutral",
        });
        if (!ok) return;
      }
      onTabChange(next);
    },
    [activeTab, hasDraft, secretDraft.key, confirm, onTabChange]
  );

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
          <div class="text-[11px] text-ink-300">Projects</div>
          <div class="text-[15px] font-semibold text-ink-50 truncate">
            {project ? `${project.name} — settings` : "Project settings"}
          </div>
        </div>
        <button
          type="button"
          onClick={onRefresh}
          disabled={refreshing}
          class="h-10 w-10 rounded-md text-ink-300 hover:text-ink-50 hover:bg-tint-strong
                 disabled:cursor-wait grid place-items-center"
          aria-label="Refresh"
          title="Refresh"
        >
          {refreshing ? <Loader class="w-4 h-4 animate-spin" /> : <RotateCcw class="w-4 h-4" />}
        </button>
      </header>

      <div class="flex-1 min-h-0 flex flex-col md:flex-row overflow-hidden">
        <ProjectSettingsNavigation
          activeTab={activeTab}
          onTabChange={handleTabChange}
          className="theme-submenu-surface hidden md:flex w-56 flex-none border-r border-line bg-inset p-3"
        />
        <ProjectSettingsNavigation
          activeTab={activeTab}
          onTabChange={handleTabChange}
          mobile
          className="theme-submenu-surface md:hidden flex-none border-b border-line bg-inset px-3 py-2 overflow-x-auto no-scrollbar"
        />

        <main
          id="project-settings-active-panel"
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

            {!project ? (
              <Empty text="Select a project from the sidebar." />
            ) : (
              <>
                {activeTab === "info" && (
                  <div class="space-y-3">
                    <ProjectHeader
                      project={project}
                      info={infoRecord.data}
                      refreshedAt={infoRecord.refreshedAt}
                      usageSummary={usageSummary}
                      usageLoading={usageLoading}
                      usageError={usageError}
                    />
                    <ProjectInfoSection
                      project={project}
                      record={infoRecord}
                      onRepairNetwork={onRepairNetwork}
                    />
                  </div>
                )}

                {activeTab === "settings" && (
                  <div class="space-y-4">
                    <ProjectResourceLimits
                      effective={infoRecord.data?.limits}
                      overrides={infoRecord.data ? infoRecord.data.limitOverrides : project.resourceLimits}
                      loading={infoRecord.loading}
                      isAdmin={isAdmin}
                      serverMemoryTotalBytes={serverMemoryTotalBytes}
                      serverMemoryLoading={serverMemoryLoading}
                      onSave={onSetResourceLimits}
                    />
                    <ProjectSettingsPanel
                      title="Project lifecycle"
                      description={`Current status: ${project.status || "unknown"}`}
                      Icon={Settings}
                    >
                      <ProjectActions
                        project={project}
                        onStart={onStartProject}
                        onStop={onStopProject}
                        onRestart={onRestartProject}
                        onDelete={onDeleteProject}
                      />
                    </ProjectSettingsPanel>
                  </div>
                )}

                {activeTab === "secrets" && (
                  <ProjectSettingsPanel
                    title="Project secrets"
                    description={secretsDescription(secretsRecord)}
                    Icon={Key}
                  >
                    <ProjectSecretsSection
                      record={secretsRecord}
                      draft={secretDraft}
                      onDraftChange={(patch) =>
                        setSecretDraft((prev) => ({ ...prev, ...patch }))
                      }
                      onSave={onSaveSecret}
                      onDelete={onDeleteSecret}
                    />
                  </ProjectSettingsPanel>
                )}

                {activeTab === "sharing" && (
                  <ProjectSettingsPanel
                    title="Project access"
                    description={accessDescription(accessRecord)}
                    Icon={Users}
                  >
                    <ProjectSharingSection
                      record={accessRecord}
                      onAdd={onAddMember}
                      onRemove={onRemoveMember}
                    />
                  </ProjectSettingsPanel>
                )}
              </>
            )}
          </div>
        </main>
      </div>
    </div>
  );
}

function ProjectSettingsNavigation({
  activeTab,
  onTabChange,
  mobile = false,
  className,
}: {
  activeTab: ProjectSettingsTab;
  onTabChange: (tab: ProjectSettingsTab) => void | Promise<void>;
  mobile?: boolean;
  className: string;
}) {
  return (
    <aside class={className} aria-label="Project settings sections">
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
              aria-controls="project-settings-active-panel"
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

function ProjectSettingsPanel({
  title,
  description,
  Icon,
  children,
}: {
  title: string;
  description: string;
  Icon: ComponentType<{ class?: string }>;
  children: ComponentChildren;
}) {
  return (
    <section class="rounded-card border border-line bg-surface overflow-hidden">
      <header class="px-4 py-3 flex items-start gap-3 border-b border-line">
        <div class="mt-0.5 grid h-8 w-8 flex-none place-items-center rounded-control bg-tint">
          <Icon class="w-4 h-4 text-ink-200" />
        </div>
        <div class="flex-1 min-w-0">
          <div class="text-[14.5px] font-semibold text-ink-50">{title}</div>
          <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">{description}</div>
        </div>
      </header>
      <div class="p-3 space-y-3">{children}</div>
    </section>
  );
}

function secretsDescription(record: SecretsRecord): string {
  if (record.loading && !record.data) return "Loading project secrets…";
  if (record.error) return "Project secrets could not be loaded.";
  const count = record.data?.length ?? 0;
  return `${count} configured secret${count === 1 ? "" : "s"}`;
}

function accessDescription(record: AccessRecord): string {
  if (record.loading && !record.data) return "Loading project members…";
  if (record.error) return "Project members could not be loaded.";
  const count = record.data?.length ?? 0;
  return `${count} project member${count === 1 ? "" : "s"}`;
}

function ProjectHeader({
  project,
  info,
  refreshedAt,
  usageSummary,
  usageLoading,
  usageError,
}: {
  project: ProjectMeta;
  info?: ProjectContainerInfo;
  refreshedAt?: number;
  usageSummary: UsageSummary | null;
  usageLoading: boolean;
  usageError: string | null;
}) {
  return (
    <section class="rounded-card border border-line bg-surface px-4 py-3 flex items-start gap-3">
      <div class="mt-0.5 grid h-8 w-8 flex-none place-items-center rounded-control bg-tint">
        <Settings class="w-4 h-4 text-ink-200" />
      </div>
      <div class="flex-1 min-w-0">
        <div class="flex items-center gap-2 min-w-0">
          <span class="text-[14.5px] font-semibold text-ink-50 truncate">{project.name}</span>
          {info && <ContainerStateBadge state={info.state ?? "UNKNOWN"} />}
        </div>
        <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug font-mono truncate">
          {project.containerName || project.slug}
        </div>
        <ProjectUsageLine summary={usageSummary} loading={usageLoading} error={usageError} />
      </div>
      {refreshedAt && (
        <div class="text-[11px] text-ink-400 mt-1.5">refreshed {fmtRelative(refreshedAt)}</div>
      )}
    </section>
  );
}

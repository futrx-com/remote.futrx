import { useState } from "preact/hooks";
import type {
	AgentContainerStatus,
  AuthBundleFileStatus,
  AuthBundleStatus,
  ContainerLimits,
  DiskUsage,
  NetworkInterface,
  OSInfo,
  ProjectContainerInfo,
  ProjectMeta,
  ResourceInfo,
  WorkspaceInfo,
} from "../../../models/project";
import type { ProjectContainerRecord } from "../../../models/project";
import { AlertCircle } from "../../primitives/icons";
import { Field, Grid, Loading, Panel } from "./ProjectContainerPrimitives";
import {
  formatBytes,
  formatDate,
  formatDuration,
  formatUnixTime,
} from "./projectContainerFormat";

export function ProjectInfoSection({
  project,
  record,
  onRepairNetwork,
}: {
  project: ProjectMeta;
  record: ProjectContainerRecord;
  onRepairNetwork: () => Promise<void>;
}) {
  if (record.error) {
    return (
      <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
        <AlertCircle class="w-4 h-4 mt-0.5 flex-none text-accent-red" />
        <div class="min-w-0">
          <div class="font-medium text-accent-red">Container endpoint unavailable</div>
          <div class="text-ink-200 mt-0.5 break-words">
            GET /api/projects/{project.id}/container returned {record.error}.
          </div>
        </div>
      </div>
    );
  }
  if (record.loading && !record.data) {
    return <Loading text="Loading container data…" />;
  }
  if (!record.data) return <Loading text="No data." />;

  const info = record.data;
  return (
    <>
      <Panel title="Overview">
        <Grid>
          <Field label="Container" value={info.name} mono />
          <Field label="State" value={info.state ?? "UNKNOWN"} mono />
          <Field label="PID" value={info.pid ? String(info.pid) : "—"} mono />
          <Field label="Processes" value={info.resources?.processes ? String(info.resources.processes) : "—"} mono />
          <Field label="Image" value={info.image || "—"} />
          <Field label="Architecture" value={info.architecture || "—"} mono />
          <Field label="Boot autostart" value={info.bootAutostart ? "yes" : "no"} mono />
          <Field label="Created" value={formatDate(info.createdAt)} mono />
          <Field label="Last used" value={formatDate(info.lastUsedAt)} mono />
        </Grid>
      </Panel>
      {info.os && <OSPanel os={info.os} />}
      {info.resources && <ResourcesPanel res={info.resources} />}
      {info.disks && info.disks.length > 0 && <DisksPanel disks={info.disks} />}
      {info.network && info.network.length > 0 && <NetworkPanel ifaces={info.network} onRepair={onRepairNetwork} />}
      {info.workspace && <WorkspacePanel ws={info.workspace} />}
      {info.limits && <LimitsPanel limits={info.limits} />}
      {agentStatuses(info).map((agent) => <AgentPanel key={agent.id} agent={agent} />)}
      {info.authBundles && info.authBundles.length > 0 && <AuthBundlesPanel bundles={info.authBundles} />}
    </>
  );
}

export function ContainerStateBadge({ state }: { state: string }) {
  const tone =
    state === "RUNNING"
      ? "text-accent-green bg-accent-green/[0.12]"
      : state === "STOPPED"
      ? "text-ink-300 bg-tint"
      : state === "MISSING"
      ? "text-accent-red bg-accent-red/[0.12]"
      : "text-ink-300 bg-tint";
  return (
    <span class={`inline-flex items-center h-5 px-1.5 rounded text-[11px] font-medium ${tone}`}>
      {state.toLowerCase()}
    </span>
  );
}

function OSPanel({ os }: { os: OSInfo }) {
  return (
    <Panel title="OS">
      <Grid>
        <Field label="Distribution" value={os.prettyName || "—"} />
        <Field label="Kernel" value={os.kernel || "—"} mono />
        <Field label="Hostname" value={os.hostname || "—"} mono />
        <Field label="CPU count" value={os.cpuCount ? String(os.cpuCount) : "—"} mono />
        <Field label="Uptime" value={os.uptimeSec ? formatDuration(os.uptimeSec) : "—"} mono />
      </Grid>
    </Panel>
  );
}

function ResourcesPanel({ res }: { res: ResourceInfo }) {
  return (
    <Panel title="Resources">
      <Grid>
        <Field label="Memory used" value={`${formatBytes(res.memoryCurrentBytes)} / ${formatBytes(res.memoryTotalBytes)}`} mono />
        <Field label="Memory peak" value={formatBytes(res.memoryPeakBytes)} mono />
        <Field label="Swap" value={formatBytes(res.swapCurrentBytes)} mono />
        <Field label="Disk (rootfs)" value={formatBytes(res.diskUsageBytes)} mono />
        <Field label="CPU time" value={res.cpuUsageSeconds ? `${res.cpuUsageSeconds.toLocaleString()} s` : "—"} mono />
        <Field label="Processes" value={res.processes ? String(res.processes) : "—"} mono />
      </Grid>
    </Panel>
  );
}

function DisksPanel({ disks }: { disks: DiskUsage[] }) {
  return (
    <Panel title="Disk usage (inside container)">
      <div class="space-y-2">
        {disks.map((disk) => {
          const percent = disk.totalBytes && disk.usedBytes != null
            ? Math.round((disk.usedBytes / disk.totalBytes) * 100)
            : null;
          return (
            <div
              key={disk.mountPath}
              class="rounded-md border border-line bg-tint px-3 py-2"
            >
              <div class="flex items-center justify-between gap-2 min-w-0">
                <div class="font-mono text-[12.5px] text-ink-100 truncate">{disk.mountPath}</div>
                <div class="text-[11px] text-ink-300 font-mono whitespace-nowrap">
                  {formatBytes(disk.usedBytes)} / {formatBytes(disk.totalBytes)}
                  {percent != null && <span class="ml-1.5 text-ink-400">({percent}%)</span>}
                </div>
              </div>
              {percent != null && (
                <div class="mt-1.5 h-1 rounded-full bg-tint overflow-hidden">
                  <div
                    class={`h-full ${percent > 85 ? "bg-accent-red" : percent > 60 ? "bg-accent-orange" : "bg-accent-green"}`}
                    style={{ width: `${Math.min(100, percent)}%` }}
                  />
                </div>
              )}
              <div class="mt-1 text-[11px] text-ink-400 font-mono truncate">
                {disk.filesystem} · avail {formatBytes(disk.availBytes)}
              </div>
            </div>
          );
        })}
      </div>
    </Panel>
  );
}

function NetworkPanel({
  ifaces,
  onRepair,
}: {
  ifaces: NetworkInterface[];
  onRepair: () => Promise<void>;
}) {
  const [repairing, setRepairing] = useState(false);
  const [repairErr, setRepairErr] = useState<string | null>(null);
  const noIPv4 = !ifaces.some((network) =>
    (network.addresses ?? []).some((address) => address.split("/")[0].includes(".")),
  );

  async function repair() {
    setRepairing(true);
    setRepairErr(null);
    try {
      await onRepair();
    } catch (error) {
      setRepairErr((error as Error).message);
    } finally {
      setRepairing(false);
    }
  }

  return (
    <Panel title="Network">
      <div class="space-y-2">
        {noIPv4 && (
          <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
            <AlertCircle class="w-4 h-4 mt-0.5 flex-none text-accent-red" />
            <div class="text-accent-red break-words">
              No IPv4 address — the container has no internet access. Use "Repair network" to re-run DHCP.
            </div>
          </div>
        )}
        {repairErr && (
          <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
            <AlertCircle class="w-4 h-4 mt-0.5 flex-none text-accent-red" />
            <div class="text-accent-red break-words">{repairErr}</div>
          </div>
        )}
        {ifaces.map((network) => (
          <div
            key={network.name}
            class="rounded-md border border-line bg-tint px-3 py-2 space-y-1"
          >
            <div class="flex items-center gap-2 min-w-0">
              <span class="font-mono text-[12.5px] text-ink-100">{network.name}</span>
              <span class="text-[11px] text-ink-400">{network.state ?? "—"}</span>
              {network.macAddress && (
                <span class="ml-auto font-mono text-[11px] text-ink-400">{network.macAddress}</span>
              )}
            </div>
            {network.addresses && network.addresses.length > 0 && (
              <div class="font-mono text-[12px] text-ink-200 break-all">
                {network.addresses.join(", ")}
              </div>
            )}
            <div class="text-[11px] text-ink-400 font-mono">
              rx {formatBytes(network.bytesReceived)} · tx {formatBytes(network.bytesSent)}
              {network.mtu ? ` · mtu ${network.mtu}` : ""}
              {network.hostName ? ` · host ${network.hostName}` : ""}
            </div>
          </div>
        ))}
      </div>
      <button
        type="button"
        onClick={() => void repair()}
        disabled={repairing}
        class="mt-2 h-9 w-full rounded-md border border-line bg-tint px-3 text-[13px] font-medium text-ink-100 hover:bg-tint-strong disabled:opacity-45 disabled:cursor-not-allowed"
        title="Re-runs DHCP on eth0 inside the container. Fixes the 'running but no internet' state."
      >
        {repairing ? "Repairing network..." : "Repair network"}
      </button>
    </Panel>
  );
}

function WorkspacePanel({ ws }: { ws: WorkspaceInfo }) {
  return (
    <Panel title="Workspace mount">
      <Grid>
        <Field label="Host source" value={ws.hostSource || "—"} mono />
        <Field label="Container path" value={ws.containerPath || "—"} mono />
      </Grid>
    </Panel>
  );
}

function LimitsPanel({ limits }: { limits: ContainerLimits }) {
  return (
    <Panel title="Resource limits">
      <Grid>
        <Field label="CPU" value={limits.cpu || "—"} mono />
        <Field label="Memory" value={limits.memory || "—"} mono />
        <Field label="Disk" value={limits.disk || "—"} mono />
      </Grid>
    </Panel>
  );
}

function AgentPanel({ agent }: { agent: AgentContainerStatus }) {
  return (
    <Panel title={`${agent.label || providerLabel(agent.id)} provisioning`}>
      <Grid>
        <Field label="CLI installed" value={agent.installed ? "yes" : "no"} mono />
        <Field label="Version" value={agent.version || "—"} mono />
		{agent.instructionsPath ? (
          <>
            <Field
              label="Instructions"
              value={agent.instructionsInstalled ? "installed" : "missing"}
              mono
            />
            <Field
              label="Instructions in sync"
              value={agent.instructionsInSync ? "yes" : "no"}
              mono
              tone={agent.instructionsInstalled && !agent.instructionsInSync ? "warn" : undefined}
            />
          </>
		) : <></>}
      </Grid>
    </Panel>
  );
}

function agentStatuses(info: ProjectContainerInfo): AgentContainerStatus[] {
  if (info.agents?.length) return info.agents;
  return [
    {
      id: "claude",
      label: "Claude Code",
      installed: info.claude.installed,
      version: info.claude.version,
      instructionsPath: "/root/.claude/CLAUDE.md",
      instructionsInstalled: info.claude.claudeMdInstalled,
      instructionsInSync: info.claude.claudeMdInSync,
    },
    {
      id: "codex",
      label: "Codex",
      installed: info.codex.installed,
      version: info.codex.version,
    },
  ];
}

function providerLabel(provider: string): string {
  return provider
    .split("-")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ") || "Agent";
}

function AuthBundlesPanel({ bundles }: { bundles: AuthBundleStatus[] }) {
  return (
    <Panel title="Auth bundles">
      <div class="space-y-3">
        {bundles.map((bundle) => (
          <div key={bundle.name} class="rounded-md border border-line bg-tint p-2.5 space-y-2">
            <div class="text-[12.5px] font-semibold text-ink-100">{bundle.name}</div>
            <div class="space-y-1.5">
              {bundle.files.map((file) => (
                <AuthFileRow key={file.containerPath} file={file} />
              ))}
            </div>
          </div>
        ))}
      </div>
    </Panel>
  );
}

function AuthFileRow({ file }: { file: AuthBundleFileStatus }) {
  const tone = file.containerNewer
    ? "text-accent-orange"
    : file.hostNewer
    ? "text-ink-300"
    : file.hostExists && file.containerExists
    ? "text-accent-green"
    : "text-ink-400";
  const label = file.containerNewer
    ? "container rotated — pending pull"
    : file.hostNewer
    ? "host newer — will push next prompt"
    : file.hostExists && file.containerExists
    ? "in sync"
    : !file.hostExists && !file.containerExists
    ? "missing on both"
    : file.hostExists
    ? "host only"
    : "container only";
  return (
    <div class="rounded border border-line bg-inset px-2.5 py-1.5">
      <div class="font-mono text-[11.5px] text-ink-200 break-all">{file.containerPath}</div>
      <div class={`text-[11px] mt-0.5 ${tone}`}>{label}</div>
      <div class="text-[10.5px] font-mono text-ink-400 mt-0.5">
        host {file.hostExists ? formatUnixTime(file.hostMtime) : "—"} · container {file.containerExists ? formatUnixTime(file.containerMtime) : "—"}
      </div>
    </div>
  );
}

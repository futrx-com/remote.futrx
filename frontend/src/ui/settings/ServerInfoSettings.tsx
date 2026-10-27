import type { ComponentChildren, ComponentType } from "preact";
import type { ServerInfo } from "../../models/serverInfo";
import {
  AlertCircle,
  Cpu,
  HardDrive,
  Loader,
  MemoryStick,
  Network,
  RotateCcw,
  Server,
} from "../primitives/icons";

export function ServerInfoSettings({
  currentEmail,
  isAdmin,
  info,
  loading,
  refreshing,
  error,
  onRefresh,
}: {
  currentEmail: string;
  isAdmin: boolean;
  info: ServerInfo | null;
  loading: boolean;
  refreshing: boolean;
  error: string | null;
  onRefresh: () => Promise<void>;
}) {
  if (loading && info == null) {
    return (
      <div class="rounded-card border border-line bg-surface px-4 py-12 flex items-center justify-center gap-2 text-[13px] text-ink-300">
        <Loader class="w-4 h-4 animate-spin" /> Loading parent server resources…
      </div>
    );
  }

  if (info == null) {
    return (
      <div class="rounded-lg border border-accent-red/25 bg-accent-red/[0.06] p-4">
        <div class="flex items-start gap-2.5 text-[13px] text-accent-red">
          <AlertCircle class="w-4 h-4 mt-0.5 flex-none" />
          <span>Could not load server information{error ? `: ${error}` : "."}</span>
        </div>
        <button
          type="button"
          onClick={() => void onRefresh()}
          class="mt-3 h-9 px-3 rounded-md bg-tint-strong hover:bg-tint-active text-[13px] text-ink-100"
        >
          Try again
        </button>
      </div>
    );
  }

  const location = typeof window === "undefined" ? null : window.location;
  const account = currentEmail || "Signed-in user";
  const access = isAdmin ? "Administrator" : "Member";

  return (
    <div class="space-y-4">
      <section class="rounded-card border border-line bg-surface overflow-hidden">
        <header class="px-4 py-3 flex items-start gap-3">
          <div class="mt-0.5 grid h-8 w-8 flex-none place-items-center rounded-control bg-tint">
            <Server class="w-4 h-4 text-ink-200" />
          </div>
          <div class="flex-1 min-w-0">
            <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
              <div class="text-[14.5px] font-semibold text-ink-50">
                {info.host.hostname || "Parent server"}
              </div>
              {info.host.appVersion && (
                <span class="text-[11px] font-mono text-ink-300 bg-tint border border-line rounded px-1.5 py-0.5">
                  {info.host.appVersion}
                </span>
              )}
              <span class="inline-flex items-center gap-1.5 text-[11px] text-accent-green">
                <span class="w-1.5 h-1.5 rounded-full bg-accent-green" /> Connected
              </span>
            </div>
            <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">
              Live host resources · updated {formatDateTime(info.collectedAt)}
            </div>
          </div>
          <button
            type="button"
            onClick={() => void onRefresh()}
            disabled={refreshing}
            class="h-9 px-2.5 rounded-md inline-flex items-center gap-2 text-[12px] text-ink-200 hover:text-ink-50 hover:bg-tint-strong disabled:opacity-60"
          >
            <RotateCcw class={`w-3.5 h-3.5 ${refreshing ? "animate-spin" : ""}`} />
            <span class="hidden sm:inline">Refresh</span>
          </button>
        </header>
        {error && (
          <div class="border-t border-accent-red/20 bg-accent-red/[0.05] px-4 py-2 text-[12px] text-accent-red">
            The last refresh failed: {error}
          </div>
        )}
      </section>

      <div class="grid gap-3 sm:grid-cols-2">
        <ResourceCard
          label="CPU usage"
          value={formatPercent(info.cpu.usagePercent)}
          detail={`${info.cpu.logicalCores} logical cores`}
          percent={info.cpu.usagePercent}
          Icon={Cpu}
        />
        <ResourceCard
          label="Memory usage"
          value={formatPercent(info.memory.usagePercent)}
          detail={`${formatBytes(info.memory.usedBytes)} of ${formatBytes(info.memory.totalBytes)}`}
          percent={info.memory.usagePercent}
          Icon={MemoryStick}
        />
        <ResourceCard
          label="Storage usage"
          value={formatPercent(info.storage.usagePercent)}
          detail={`${formatBytes(info.storage.usedBytes)} of ${formatBytes(info.storage.totalBytes)}`}
          percent={info.storage.usagePercent}
          Icon={HardDrive}
        />
        <ResourceCard
          label="Network transfer"
          value={`↓ ${formatBytes(info.network.receivedBytes)}`}
          detail={`↑ ${formatBytes(info.network.sentBytes)} sent since boot`}
          Icon={Network}
        />
      </div>

      <InfoSection title="Host" Icon={Server}>
        <dl class="grid sm:grid-cols-2">
          <InfoField label="Server URL" value={location?.origin || "—"} mono />
          <InfoField label="Hostname" value={info.host.hostname || "—"} mono />
          <InfoField label="Operating system" value={info.host.platform || info.host.os} />
          <InfoField label="Architecture" value={info.host.architecture} mono />
          <InfoField label="Kernel" value={info.host.kernel || "Unavailable"} mono />
          <InfoField label="App version" value={info.host.appVersion || "—"} mono />
          <InfoField label="Go runtime" value={info.host.goVersion} mono />
          <InfoField label="Host uptime" value={formatDuration(info.host.uptimeSec)} />
          <InfoField label="Service uptime" value={formatDuration(info.host.serviceUptimeSec)} />
          <InfoField label="Booted" value={formatDateTime(info.host.bootedAt)} />
          <InfoField label="Account" value={`${account} · ${access}`} />
          <InfoField label="Application data" value={info.host.dataPath} mono />
          <InfoField label="Project workspaces" value={info.host.workspacePath} mono />
        </dl>
      </InfoSection>

      <div class="grid gap-4 lg:grid-cols-2">
        <InfoSection title="Processor" Icon={Cpu}>
          <dl class="grid grid-cols-2">
            <InfoField label="Current usage" value={formatPercent(info.cpu.usagePercent)} />
            <InfoField label="Logical cores" value={String(info.cpu.logicalCores)} />
            <InfoField label="Load · 1 min" value={formatNumber(info.cpu.loadAverage1)} />
            <InfoField label="Load · 5 min" value={formatNumber(info.cpu.loadAverage5)} />
            <InfoField label="Load · 15 min" value={formatNumber(info.cpu.loadAverage15)} />
            <InfoField label="Model" value={info.cpu.model || "Unavailable"} />
          </dl>
        </InfoSection>

        <InfoSection title="Memory" Icon={MemoryStick}>
          <dl class="grid grid-cols-2">
            <InfoField label="Used" value={formatBytes(info.memory.usedBytes)} />
            <InfoField label="Total" value={formatBytes(info.memory.totalBytes)} />
            <InfoField label="Available" value={formatBytes(info.memory.availableBytes)} />
            <InfoField label="Free" value={formatBytes(info.memory.freeBytes)} />
            <InfoField label="Cached" value={formatBytes(info.memory.cachedBytes)} />
            <InfoField label="Buffers" value={formatBytes(info.memory.buffersBytes)} />
            <InfoField
              label="Swap used"
              value={`${formatBytes(info.memory.swapUsedBytes)} / ${formatBytes(info.memory.swapTotalBytes)}`}
            />
            <InfoField label="Swap free" value={formatBytes(info.memory.swapFreeBytes)} />
          </dl>
        </InfoSection>
      </div>

      <InfoSection title="Storage" Icon={HardDrive}>
        {info.storage.mounts.length === 0 ? (
          <EmptyInfo>Filesystem metrics are unavailable on this host.</EmptyInfo>
        ) : (
          <div class="divide-y divide-line">
            {info.storage.mounts.map((mount) => (
              <div key={`${mount.device}:${mount.mountPath}`} class="p-4">
                <div class="flex items-start justify-between gap-4">
                  <div class="min-w-0">
                    <div class="font-mono text-[13px] text-ink-100 truncate" title={mount.mountPath}>
                      {mount.mountPath}
                    </div>
                    <div class="mt-0.5 text-[11.5px] text-ink-400 truncate">
                      {[mount.device, mount.filesystem].filter(Boolean).join(" · ") || "Filesystem"}
                    </div>
                  </div>
                  <div class="text-right flex-none">
                    <div class="text-[13px] text-ink-100">{formatPercent(mount.usagePercent)}</div>
                    <div class="text-[11.5px] text-ink-400">
                      {formatBytes(mount.usedBytes)} / {formatBytes(mount.totalBytes)}
                    </div>
                  </div>
                </div>
                <UsageBar percent={mount.usagePercent} />
                <div class="mt-1.5 text-[11px] text-ink-400">
                  {formatBytes(mount.availableBytes)} available
                </div>
              </div>
            ))}
          </div>
        )}
      </InfoSection>

      <InfoSection title="Network" Icon={Network}>
        {info.network.interfaces.length === 0 ? (
          <EmptyInfo>Network interface metrics are unavailable on this host.</EmptyInfo>
        ) : (
          <div class="divide-y divide-line">
            {info.network.interfaces.map((networkInterface) => (
              <div key={networkInterface.name} class="px-4 py-3 flex items-start gap-3">
                <span
                  class={`mt-1.5 w-2 h-2 rounded-full flex-none ${
                    networkInterface.up ? "bg-accent-green" : "bg-ink-500"
                  }`}
                />
                <div class="flex-1 min-w-0">
                  <div class="flex flex-wrap items-center gap-x-2">
                    <span class="font-mono text-[13px] text-ink-100">{networkInterface.name}</span>
                    <span class="text-[11px] text-ink-400">
                      {networkInterface.loopback ? "loopback" : networkInterface.up ? "up" : "down"}
                    </span>
                  </div>
                  <div class="mt-1 font-mono text-[11px] leading-relaxed text-ink-400 break-all">
                    {networkInterface.addresses?.join(" · ") || "No addresses"}
                  </div>
                  <div class="mt-0.5 text-[11px] text-ink-400">
                    {[
                      networkInterface.hardwareAddress,
                      networkInterface.mtu ? `MTU ${networkInterface.mtu}` : "",
                    ].filter(Boolean).join(" · ")}
                  </div>
                </div>
                <div class="flex-none text-right text-[11.5px] text-ink-300">
                  <div>↓ {formatBytes(networkInterface.receivedBytes)}</div>
                  <div>↑ {formatBytes(networkInterface.sentBytes)}</div>
                </div>
              </div>
            ))}
          </div>
        )}
      </InfoSection>

      <InfoSection title="Remote process" Icon={Server}>
        <dl class="grid sm:grid-cols-3">
          <InfoField label="Process ID" value={String(info.process.pid)} mono />
          <InfoField label="Goroutines" value={String(info.process.goroutines)} />
          <InfoField
            label="Open file handles"
            value={info.process.openFileHandles == null ? "Unavailable" : String(info.process.openFileHandles)}
          />
          <InfoField label="Allocated memory" value={formatBytes(info.process.allocatedBytes)} />
          <InfoField label="Heap in use" value={formatBytes(info.process.heapInUseBytes)} />
          <InfoField label="Go system memory" value={formatBytes(info.process.systemMemoryBytes)} />
        </dl>
      </InfoSection>
    </div>
  );
}

function ResourceCard({
  label,
  value,
  detail,
  percent,
  Icon,
}: {
  label: string;
  value: string;
  detail: string;
  percent?: number;
  Icon: ComponentType<{ class?: string }>;
}) {
  return (
    <section class="rounded-card border border-line bg-surface p-4">
      <div class="flex items-center gap-2 text-[12px] text-ink-300">
        <Icon class="w-4 h-4" /> {label}
      </div>
      <div class="mt-2 text-xl font-semibold text-ink-50">{value}</div>
      <div class="mt-0.5 text-[11.5px] text-ink-400">{detail}</div>
      {percent != null && <UsageBar percent={percent} />}
    </section>
  );
}

function InfoSection({
  title,
  Icon,
  children,
}: {
  title: string;
  Icon: ComponentType<{ class?: string }>;
  children: ComponentChildren;
}) {
  return (
    <section class="rounded-card border border-line bg-surface overflow-hidden">
      <header class="px-4 py-3 border-b border-line flex items-center gap-2">
        <Icon class="w-4 h-4 text-ink-300" />
        <h2 class="text-[14px] font-semibold text-ink-50">{title}</h2>
      </header>
      {children}
    </section>
  );
}

function InfoField({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div class="min-w-0 px-4 py-3 border-b border-line sm:border-r">
      <dt class="text-[10.5px] uppercase tracking-wide text-ink-400">{label}</dt>
      <dd
        class={`mt-1 text-[12.5px] text-ink-100 break-words ${mono ? "font-mono" : ""}`}
        title={value}
      >
        {value}
      </dd>
    </div>
  );
}

function UsageBar({ percent }: { percent: number }) {
  const normalized = Math.max(0, Math.min(percent, 100));
  const color = normalized >= 90 ? "bg-accent-red" : normalized >= 75 ? "bg-accent-yellow" : "bg-accent-blue";
  return (
    <div class="mt-3 h-1.5 rounded-full bg-tint-strong overflow-hidden">
      <div class={`h-full rounded-full ${color}`} style={{ width: `${normalized}%` }} />
    </div>
  );
}

function EmptyInfo({ children }: { children: ComponentChildren }) {
  return <div class="px-4 py-5 text-[12.5px] text-ink-400">{children}</div>;
}

function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  const amount = value / 1024 ** index;
  return `${amount >= 10 || index === 0 ? amount.toFixed(0) : amount.toFixed(1)} ${units[index]}`;
}

function formatPercent(value?: number): string {
  return value == null ? "Unavailable" : `${value.toFixed(1)}%`;
}

function formatNumber(value?: number): string {
  return value == null ? "Unavailable" : value.toFixed(2);
}

function formatDuration(seconds?: number): string {
  if (seconds == null || seconds <= 0) return "Unavailable";
  if (seconds < 60) return `${Math.floor(seconds)}s`;
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return [days ? `${days}d` : "", hours ? `${hours}h` : "", `${minutes}m`].filter(Boolean).join(" ");
}

function formatDateTime(unixSeconds?: number): string {
  if (unixSeconds == null || unixSeconds <= 0) return "Unavailable";
  return new Date(unixSeconds * 1000).toLocaleString();
}

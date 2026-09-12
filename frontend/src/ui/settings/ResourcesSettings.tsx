import { useEffect, useMemo, useState } from "preact/hooks";
import type { FleetResourcesView, FleetSettings } from "../../models/resources";
import {
  formatSize,
  hostCommitmentPercent,
  validateFleetSettings,
} from "../../state/settings/resourcePolicyState";
import { AlertCircle, Cpu, HardDrive, Loader, MemoryStick, Server } from "../primitives/icons";
import { Meter } from "../primitives/Meter";

/**
 * Fleet resource policy. Everything here used to be compiled into the binary,
 * so raising the fleet ceiling meant editing Go source and redeploying.
 */
export function ResourcesSettings({
  view,
  loading,
  saving,
  error,
  onSave,
}: {
  view: FleetResourcesView | null;
  loading: boolean;
  saving: boolean;
  error: string | null;
  onSave: (settings: Partial<FleetSettings>) => Promise<void>;
}) {
  const [memory, setMemory] = useState("");
  const [cpu, setCPU] = useState("");
  const [processes, setProcesses] = useState("");
  const [disk, setDisk] = useState("");
  const [reserve, setReserve] = useState("");
  const [maxMemory, setMaxMemory] = useState("");
  const [maxCPU, setMaxCPU] = useState("");
  const [maxDisk, setMaxDisk] = useState("");
  const [maxRunning, setMaxRunning] = useState("0");
  const [saved, setSaved] = useState(false);

  const settings = view?.settings;
  useEffect(() => {
    if (!settings) return;
    setMemory(settings.defaults.memory ?? "");
    setCPU(String(settings.defaults.cpu ?? ""));
    setProcesses(String(settings.defaults.processes ?? ""));
    setDisk(settings.defaults.disk ?? "");
    setReserve(settings.hostReserve.memory ?? "");
    setMaxMemory(settings.maxProjectOverride.memory ?? "");
    setMaxCPU(settings.maxProjectOverride.cpu ? String(settings.maxProjectOverride.cpu) : "");
    setMaxDisk(settings.maxProjectOverride.disk ?? "");
    setMaxRunning(String(settings.maxRunningContainers ?? 0));
  }, [settings]);

  const draft = useMemo(
    () => ({
      defaults: {
        memory: memory.trim(),
        cpu: Number(cpu),
        processes: Number(processes),
        disk: disk.trim(),
      },
      maxProjectOverride: {
        memory: maxMemory.trim(),
        cpu: maxCPU.trim() ? Number(maxCPU) : 0,
        disk: maxDisk.trim(),
      },
      maxRunningContainers: Number(maxRunning),
    }),
    [memory, cpu, processes, disk, maxMemory, maxCPU, maxDisk, maxRunning]
  );

  const validationError = useMemo(
    () =>
      validateFleetSettings(
        draft.defaults,
        reserve,
        draft.maxProjectOverride,
        draft.maxRunningContainers,
        view?.host
      ),
    [draft, reserve, view?.host]
  );

  if (loading && view == null) {
    return (
      <div class="rounded-lg border border-white/10 bg-[#101318] px-4 py-12 flex items-center justify-center gap-2 text-[13px] text-ink-300">
        <Loader class="w-4 h-4 animate-spin" /> Loading resource policy…
      </div>
    );
  }

  const submit = (event: Event) => {
    event.preventDefault();
    if (validationError) return;
    setSaved(false);
    void onSave({
      defaults: draft.defaults,
      hostReserve: { memory: reserve.trim() },
      maxProjectOverride: draft.maxProjectOverride,
      maxRunningContainers: draft.maxRunningContainers,
    })
      .then(() => setSaved(true))
      .catch(() => setSaved(false));
  };

  const host = view?.host;
  const committed = hostCommitmentPercent(host);

  return (
    <div class="space-y-4">
      <Panel
        Icon={Server}
        title="Host capacity"
        description={
          view?.runtimeAvailable === false
            ? "The container runtime is unreachable, so live commitment cannot be measured."
            : "What this machine has, and how much of it running workspaces already claim."
        }
      >
        <div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
          <Stat Icon={MemoryStick} label="Total memory" value={formatSize(host?.memoryBytes)} />
          <Stat Icon={MemoryStick} label="Host reserve" value={formatSize(host?.reserveMemoryBytes)} />
          <Stat Icon={MemoryStick} label="Workspace budget" value={formatSize(host?.budgetMemoryBytes)} />
          <Stat Icon={Cpu} label="Logical CPUs" value={host?.cpus ? String(host.cpus) : "—"} />
        </div>
        <Meter
          label={`Committed by ${host?.runningContainers ?? 0} running container${host?.runningContainers === 1 ? "" : "s"}`}
          detail={`${formatSize(host?.committedMemoryBytes)} of ${formatSize(host?.budgetMemoryBytes)}`}
          percent={committed}
        />
        <div class="rounded-md border border-white/[0.08] bg-white/[0.03] px-3 py-2.5 text-[12px] leading-relaxed text-ink-300">
          <span class="inline-flex items-center gap-1.5 text-ink-100">
            <HardDrive class="w-3.5 h-3.5" /> Disk quotas
          </span>
          {" · "}
          {view?.diskQuota.supported ? (
            <>
              supported on the <span class="font-mono text-ink-100">{view.diskQuota.driver}</span> pool
              {view.diskQuota.pool ? ` "${view.diskQuota.pool}"` : ""}.
            </>
          ) : (
            <>
              unsupported on this pool
              {view?.diskQuota.driver ? ` (${view.diskQuota.driver})` : ""}.{" "}
              {view?.diskQuota.detail || "A disk quota set here is recorded but not enforced."}
            </>
          )}
        </div>
      </Panel>

      <Panel
        Icon={Cpu}
        title="Fleet defaults"
        description={
          settings?.derived
            ? "Derived from this host's capacity on first run. Every project without an override inherits these."
            : "Every project without an override inherits these. Changes converge onto running containers immediately."
        }
      >
        <form class="space-y-4" onSubmit={submit}>
          <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <Field label="Memory" value={memory} onInput={setMemory} hint="For example 2GiB" />
            <Field label="CPU cores" value={cpu} onInput={setCPU} hint="Whole number" />
            <Field label="Process limit" value={processes} onInput={setProcesses} hint="Fork-bomb guard" />
            <Field
              label="Disk quota"
              value={disk}
              onInput={setDisk}
              hint={view?.diskQuota.supported ? "For example 20GiB" : "Not enforced on this pool"}
            />
          </div>

          <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <Field
              label="Host reserve"
              value={reserve}
              onInput={setReserve}
              hint="Memory held back for the platform"
            />
            <Field label="Max project memory" value={maxMemory} onInput={setMaxMemory} hint="Override ceiling" />
            <Field label="Max project CPU" value={maxCPU} onInput={setMaxCPU} hint="Override ceiling" />
            <Field label="Max project disk" value={maxDisk} onInput={setMaxDisk} hint="Override ceiling" />
          </div>

          <div class="grid gap-3 md:grid-cols-2">
            <Field
              label="Max running containers"
              value={maxRunning}
              onInput={setMaxRunning}
              hint="0 means unlimited"
            />
          </div>

          <div class="flex items-start gap-2 rounded-md border border-accent-orange/25 bg-accent-orange/[0.07] px-3 py-2.5 text-[12px] leading-relaxed text-ink-200">
            <AlertCircle class="mt-0.5 w-4 h-4 flex-none text-accent-orange" />
            <span>
              Starting a workspace is refused when the running containers would commit more
              than the workspace budget. Lowering the default memory can trigger the
              container's own OOM killer inside an active workspace.
            </span>
          </div>

          {(validationError || error) && (
            <div class="rounded-md border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2 text-[12.5px] text-accent-red">
              {validationError || error}
            </div>
          )}
          {saved && !error && !validationError && (
            <div class="rounded-md border border-accent-blue/30 bg-accent-blue/[0.08] px-3 py-2 text-[12.5px] text-ink-100">
              Saved. The managed LXD profile has been converged onto the new defaults.
            </div>
          )}

          <div class="flex justify-end">
            <button
              type="submit"
              disabled={saving || !!validationError}
              class="h-9 px-4 rounded-md bg-accent-blue/80 hover:bg-accent-blue text-white text-[13px] font-medium disabled:opacity-50 inline-flex items-center justify-center gap-2"
            >
              {saving && <Loader class="w-3.5 h-3.5 animate-spin" />}
              Save fleet defaults
            </button>
          </div>
        </form>
      </Panel>
    </div>
  );
}

function Panel({
  Icon,
  title,
  description,
  children,
}: {
  Icon: (props: { class?: string }) => preact.JSX.Element;
  title: string;
  description: string;
  children: preact.ComponentChildren;
}) {
  return (
    <section class="rounded-lg border border-white/10 bg-[#101318] overflow-hidden">
      <header class="px-4 py-3 flex items-start gap-3 border-b border-white/[0.06]">
        <div class="mt-0.5 w-9 h-9 rounded-md bg-white/[0.06] border border-white/10 grid place-items-center flex-none">
          <Icon class="w-4 h-4 text-ink-200" />
        </div>
        <div class="flex-1 min-w-0">
          <div class="text-[14.5px] font-semibold text-ink-50">{title}</div>
          <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">{description}</div>
        </div>
      </header>
      <div class="p-4 space-y-4">{children}</div>
    </section>
  );
}

function Stat({
  Icon,
  label,
  value,
}: {
  Icon: (props: { class?: string }) => preact.JSX.Element;
  label: string;
  value: string;
}) {
  return (
    <div class="rounded-md border border-white/[0.08] bg-white/[0.03] px-3 py-2.5">
      <div class="flex items-center gap-1.5 text-[11px] text-ink-400">
        <Icon class="w-3.5 h-3.5" /> {label}
      </div>
      <div class="mt-1 font-mono text-[13px] text-ink-100">{value}</div>
    </div>
  );
}

function Field({
  label,
  value,
  hint,
  onInput,
}: {
  label: string;
  value: string;
  hint: string;
  onInput: (value: string) => void;
}) {
  return (
    <label class="block min-w-0">
      <span class="block text-[12.5px] font-medium text-ink-100">{label}</span>
      <input
        type="text"
        value={value}
        onInput={(event) => onInput(event.currentTarget.value)}
        spellcheck={false}
        class="mt-1.5 h-10 w-full rounded-md border border-white/10 bg-[#0c0f13] px-3 font-mono text-[13px] text-ink-50 outline-none placeholder:text-ink-500 focus:border-accent-blue/60 focus:ring-1 focus:ring-accent-blue/25"
      />
      <span class="mt-1 block text-[11px] text-ink-400">{hint}</span>
    </label>
  );
}

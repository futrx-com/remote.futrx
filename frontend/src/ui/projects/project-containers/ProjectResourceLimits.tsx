import { useEffect, useMemo, useState } from "preact/hooks";
import type { ContainerLimits } from "../../../models/project";
import type { ProjectResources } from "../../../models/resources";
import {
  formatSize,
  usageMeters,
  validateOverride,
} from "../../../state/settings/resourcePolicyState";
import { AlertCircle, Cpu, HardDrive, Loader, MemoryStick, RotateCcw } from "../../primitives/icons";
import { Meter } from "../../primitives/Meter";

/**
 * Per-project resource envelope: what LXD enforces today, live consumption
 * against those limits, and the admin-only form for overriding the fleet
 * defaults within the operator's ceiling.
 */
export function ProjectResourceLimits({
  resources,
  loading,
  saving,
  error,
  onSave,
}: {
  resources: ProjectResources | null;
  loading: boolean;
  saving: boolean;
  error: string | null;
  onSave: (limits: ContainerLimits) => Promise<void>;
}) {
  const [cpu, setCPU] = useState("");
  const [memory, setMemory] = useState("");
  const [disk, setDisk] = useState("");

  const overrides = resources?.overrides;
  useEffect(() => {
    setCPU(overrides?.cpu ?? "");
    setMemory(overrides?.memory ?? "");
    setDisk(overrides?.disk ?? "");
  }, [overrides?.cpu, overrides?.memory, overrides?.disk]);

  const policy = resources?.policy;
  const effective = resources?.effective ?? {};
  const validationError = useMemo(
    () => validateOverride({ cpu, memory, disk }, policy?.maxOverride),
    [cpu, memory, disk, policy?.maxOverride]
  );
  const meters = useMemo(() => usageMeters(resources?.usage, effective), [resources]);
  const editable = resources?.editable ?? false;
  const quotaSupported = policy?.diskQuota.supported ?? false;

  const submit = (event: Event) => {
    event.preventDefault();
    if (validationError) return;
    void onSave({ cpu: cpu.trim(), memory: memory.trim(), disk: disk.trim() }).catch(() => {});
  };

  const reset = () => {
    setCPU("");
    setMemory("");
    setDisk("");
    void onSave({}).catch(() => {});
  };

  return (
    <section class="rounded-lg border border-white/10 bg-[#101318] overflow-hidden">
      <header class="px-4 py-3 flex items-start gap-3 border-b border-white/[0.06]">
        <div class="mt-0.5 w-9 h-9 rounded-md bg-white/[0.06] border border-white/10 grid place-items-center flex-none">
          <Cpu class="w-4 h-4 text-ink-200" />
        </div>
        <div class="flex-1 min-w-0">
          <div class="text-[14.5px] font-semibold text-ink-50">Resources</div>
          <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">
            Leave a field blank to inherit the fleet default set in Settings → Resources.
          </div>
        </div>
      </header>

      <div class="p-4 space-y-4">
        <div class="grid gap-2 sm:grid-cols-3">
          <Effective
            Icon={Cpu}
            label="CPU"
            value={effective.cpu || "—"}
            note={inheritedNote(overrides?.cpu, policy?.defaults.cpu)}
          />
          <Effective
            Icon={MemoryStick}
            label="Memory"
            value={effective.memory || "—"}
            note={inheritedNote(overrides?.memory, policy?.defaults.memory)}
          />
          <Effective
            Icon={HardDrive}
            label="Disk quota"
            value={effective.disk || "No quota"}
            note={
              quotaSupported
                ? inheritedNote(overrides?.disk, policy?.defaults.disk)
                : "not enforced on this pool"
            }
          />
        </div>

        {loading && !resources ? (
          <div class="flex items-center gap-2 text-[12.5px] text-ink-300">
            <Loader class="w-4 h-4 animate-spin" /> Loading current limits…
          </div>
        ) : (
          <div class="space-y-3 rounded-md border border-white/[0.08] bg-white/[0.03] px-3 py-3">
            {meters.map((usage) => (
              <Meter
                key={usage.label}
                label={`${usage.label} usage`}
                detail={usage.detail}
                percent={usage.percent}
              />
            ))}
            <div class="text-[11px] text-ink-400">
              Container is {resources?.state ?? "UNKNOWN"} · host has{" "}
              {formatSize(policy?.host.budgetMemoryBytes)} for workspaces, of which{" "}
              {formatSize(policy?.host.committedMemoryBytes)} is committed by{" "}
              {policy?.host.runningContainers ?? 0} running container
              {policy?.host.runningContainers === 1 ? "" : "s"}.
            </div>
          </div>
        )}

        {!editable ? (
          <div class="rounded-md border border-white/10 bg-white/[0.03] px-3 py-2.5 text-[12.5px] text-ink-300">
            Only an administrator can change container resources.
          </div>
        ) : (
          <form class="space-y-4" onSubmit={submit}>
            <div class="grid gap-3 md:grid-cols-3">
              <LimitInput
                label="CPU cores"
                value={cpu}
                placeholder={policy?.defaults.cpu || "Fleet default"}
                hint={maxHint("Max", policy?.maxOverride.cpu)}
                onInput={setCPU}
              />
              <LimitInput
                label="Memory"
                value={memory}
                placeholder={policy?.defaults.memory || "Fleet default"}
                hint={maxHint("Max", policy?.maxOverride.memory)}
                onInput={setMemory}
              />
              <LimitInput
                label="Disk quota"
                value={disk}
                placeholder={policy?.defaults.disk || "No quota"}
                hint={
                  quotaSupported
                    ? maxHint("Max", policy?.maxOverride.disk)
                    : "Unsupported on this storage pool"
                }
                onInput={setDisk}
              />
            </div>

            <div class="flex items-start gap-2 rounded-md border border-accent-orange/25 bg-accent-orange/[0.07] px-3 py-2.5 text-[12px] leading-relaxed text-ink-200">
              <AlertCircle class="mt-0.5 w-4 h-4 flex-none text-accent-orange" />
              <span>
                CPU and memory apply live; lowering memory can stop processes inside the
                container. A disk quota cannot be smaller than the data already stored.
              </span>
            </div>

            {(validationError || error) && (
              <div class="rounded-md border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2 text-[12.5px] text-accent-red">
                {validationError || error}
              </div>
            )}
            {resources?.needsRestart && !error && (
              <div class="rounded-md border border-accent-orange/30 bg-accent-orange/[0.08] px-3 py-2 text-[12.5px] text-ink-100">
                The disk quota is recorded but takes effect on the next container restart.
              </div>
            )}
            {resources?.appliedNow && !resources.needsRestart && !error && (
              <div class="rounded-md border border-accent-blue/30 bg-accent-blue/[0.08] px-3 py-2 text-[12.5px] text-ink-100">
                Applied to the running container.
              </div>
            )}

            <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
              <button
                type="button"
                onClick={reset}
                disabled={saving}
                class="h-9 px-3 rounded-md border border-white/10 text-[13px] font-medium text-ink-200 hover:bg-white/[0.06] disabled:opacity-50 inline-flex items-center justify-center gap-2"
              >
                <RotateCcw class="w-3.5 h-3.5" /> Reset to defaults
              </button>
              <button
                type="submit"
                disabled={saving || !!validationError}
                class="h-9 px-4 rounded-md bg-accent-blue/80 hover:bg-accent-blue text-white text-[13px] font-medium disabled:opacity-50 inline-flex items-center justify-center gap-2"
              >
                {saving && <Loader class="w-3.5 h-3.5 animate-spin" />}
                Save limits
              </button>
            </div>
          </form>
        )}
      </div>
    </section>
  );
}

function inheritedNote(override?: string, fleetDefault?: string): string {
  if (override && override.trim()) return "project override";
  return fleetDefault ? `fleet default ${fleetDefault}` : "no fleet default";
}

function maxHint(prefix: string, ceiling?: string): string {
  return ceiling ? `${prefix} ${ceiling}` : "No fleet ceiling set";
}

function Effective({
  Icon,
  label,
  value,
  note,
}: {
  Icon: (props: { class?: string }) => preact.JSX.Element;
  label: string;
  value: string;
  note: string;
}) {
  return (
    <div class="rounded-md border border-white/[0.08] bg-white/[0.03] px-3 py-2.5">
      <div class="flex items-center gap-1.5 text-[11px] text-ink-400">
        <Icon class="w-3.5 h-3.5" /> {label}
      </div>
      <div class="mt-1 font-mono text-[13px] text-ink-100">{value}</div>
      <div class="mt-0.5 text-[11px] text-ink-400">{note}</div>
    </div>
  );
}

function LimitInput({
  label,
  value,
  placeholder,
  hint,
  onInput,
}: {
  label: string;
  value: string;
  placeholder: string;
  hint: string;
  onInput: (value: string) => void;
}) {
  return (
    <label class="block min-w-0">
      <span class="block text-[12.5px] font-medium text-ink-100">{label}</span>
      <input
        type="text"
        value={value}
        placeholder={placeholder}
        onInput={(event) => onInput(event.currentTarget.value)}
        spellcheck={false}
        class="mt-1.5 h-10 w-full rounded-md border border-white/10 bg-[#0c0f13] px-3 font-mono text-[13px] text-ink-50 outline-none placeholder:text-ink-500 focus:border-accent-blue/60 focus:ring-1 focus:ring-accent-blue/25"
      />
      <span class="mt-1 block text-[11px] text-ink-400">{hint}</span>
    </label>
  );
}

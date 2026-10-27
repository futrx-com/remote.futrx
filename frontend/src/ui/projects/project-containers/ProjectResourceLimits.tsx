import { useEffect, useMemo, useState } from "preact/hooks";
import type { ContainerLimits } from "../../../models/project";
import { AlertCircle, Cpu, HardDrive, Loader, MemoryStick, RotateCcw } from "../../primitives/icons";
import { formatBytes } from "./projectContainerFormat";

const sizePattern = /^[1-9][0-9]*(MiB|GiB|TiB)$/;

export function ProjectResourceLimits({
  effective,
  overrides,
  loading,
  isAdmin,
  serverMemoryTotalBytes,
  serverMemoryLoading,
  onSave,
}: {
  effective?: ContainerLimits;
  overrides?: ContainerLimits;
  loading: boolean;
  isAdmin: boolean;
  serverMemoryTotalBytes?: number;
  serverMemoryLoading: boolean;
  onSave: (limits: ContainerLimits) => Promise<void>;
}) {
  const [cpu, setCPU] = useState("");
  const [memory, setMemory] = useState("");
  const [disk, setDisk] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();

  useEffect(() => {
    setCPU(overrides?.cpu ?? "");
    setMemory(overrides?.memory ?? "");
    setDisk(overrides?.disk ?? "");
  }, [overrides?.cpu, overrides?.memory, overrides?.disk]);

  const validationError = useMemo(() => {
    const trimmedCPU = cpu.trim();
    if (trimmedCPU) {
      const cores = Number(trimmedCPU);
      if (!Number.isInteger(cores) || cores < 1 || cores > 256) {
        return "CPU must be a whole number from 1 to 256.";
      }
    }
    if (memory.trim() && !sizePattern.test(memory.trim())) {
      return "Memory must use MiB, GiB, or TiB, for example 8GiB.";
    }
    if (disk.trim() && !sizePattern.test(disk.trim())) {
      return "Disk must use MiB, GiB, or TiB, for example 40GiB.";
    }
    return undefined;
  }, [cpu, memory, disk]);

  const save = async (limits: ContainerLimits) => {
    setSaving(true);
    setError(undefined);
    try {
      await onSave(limits);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const submit = (event: Event) => {
    event.preventDefault();
    if (validationError) return;
    void save({
      cpu: cpu.trim(),
      memory: memory.trim(),
      disk: disk.trim(),
    });
  };

  const reset = () => {
    setCPU("");
    setMemory("");
    setDisk("");
    void save({});
  };

  return (
    <section class="rounded-card border border-line bg-surface overflow-hidden">
      <header class="px-4 py-3 flex items-start gap-3 border-b border-line">
        <div class="mt-0.5 grid h-8 w-8 flex-none place-items-center rounded-control bg-tint">
          <Cpu class="w-4 h-4 text-ink-200" />
        </div>
        <div class="flex-1 min-w-0">
          <div class="text-[14.5px] font-semibold text-ink-50">Resource limits</div>
          <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">
            Define the CPU, memory, and storage available to this container.
          </div>
        </div>
      </header>

      <div class="p-4 space-y-4">
        <div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
          <EffectiveLimit Icon={Cpu} label="Effective CPU" value={effective?.cpu || "Inherited"} />
          <EffectiveLimit Icon={MemoryStick} label="Effective memory" value={effective?.memory || "Inherited"} />
          <EffectiveLimit Icon={HardDrive} label="Effective disk quota" value={effective?.disk || "No quota"} />
          <EffectiveLimit
            Icon={MemoryStick}
            label="Server total memory"
            value={serverMemoryLoading ? "Loading…" : formatBytes(serverMemoryTotalBytes)}
          />
        </div>

        {loading && !effective ? (
          <div class="flex items-center gap-2 text-[12.5px] text-ink-300">
            <Loader class="w-4 h-4 animate-spin" /> Loading current limits…
          </div>
        ) : !isAdmin ? (
          <div class="rounded-md border border-line bg-tint px-3 py-2.5 text-[12.5px] text-ink-300">
            Only an administrator can change container resources.
          </div>
        ) : (
          <form class="space-y-4" onSubmit={submit}>
            <div class="grid gap-3 md:grid-cols-3">
              <LimitInput
                label="CPU cores"
                value={cpu}
                placeholder={effective?.cpu || "Fleet default"}
                hint="Whole number from 1–256"
                onInput={setCPU}
              />
              <LimitInput
                label="Memory"
                value={memory}
                placeholder={effective?.memory || "Fleet default"}
                hint="For example 8GiB"
                onInput={setMemory}
              />
              <LimitInput
                label="Disk quota"
                value={disk}
                placeholder={effective?.disk || "No quota"}
                hint="For example 40GiB"
                onInput={setDisk}
              />
            </div>

            <div class="flex items-start gap-2 rounded-md border border-accent-orange/25 bg-accent-orange/[0.07] px-3 py-2.5 text-[12px] leading-relaxed text-ink-200">
              <AlertCircle class="mt-0.5 w-4 h-4 flex-none text-accent-orange" />
              <span>
                Changes apply live. Lowering memory can stop container processes, and a disk quota cannot be smaller than the data already stored. Leave a field blank to inherit the fleet default.
              </span>
            </div>

            {(validationError || error) && (
              <div class="rounded-md border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2 text-[12.5px] text-accent-red">
                {validationError || error}
              </div>
            )}

            <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
              <button
                type="button"
                onClick={reset}
                disabled={saving}
                class="h-9 px-3 rounded-md border border-line text-[13px] font-medium text-ink-200 hover:bg-tint disabled:opacity-50 inline-flex items-center justify-center gap-2"
              >
                <RotateCcw class="w-3.5 h-3.5" /> Reset to defaults
              </button>
              <button
                type="submit"
                disabled={saving || !!validationError}
                class="btn btn-primary btn-sm text-[13px] font-medium disabled:opacity-50 inline-flex items-center justify-center gap-2"
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

function EffectiveLimit({
  Icon,
  label,
  value,
}: {
  Icon: (props: { class?: string }) => preact.JSX.Element;
  label: string;
  value: string;
}) {
  return (
    <div class="rounded-md border border-line bg-tint px-3 py-2.5">
      <div class="flex items-center gap-1.5 text-[11px] text-ink-400">
        <Icon class="w-3.5 h-3.5" /> {label}
      </div>
      <div class="mt-1 font-mono text-[13px] text-ink-100">{value}</div>
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
        class="mt-1.5 h-10 w-full rounded-md border border-line bg-inset px-3 font-mono text-[13px] text-ink-50 outline-none placeholder:text-ink-500 focus:border-accent-blue/60 focus:ring-1 focus:ring-accent-blue/25"
      />
      <span class="mt-1 block text-[11px] text-ink-400">{hint}</span>
    </label>
  );
}

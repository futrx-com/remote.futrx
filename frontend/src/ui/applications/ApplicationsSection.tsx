import type { ComponentChildren } from "preact";
import { useMemo, useState } from "preact/hooks";
import type {
  AppCredentials,
  AppImage,
  AppInstance,
  AppInstanceStatus,
} from "../../models/application";
import type { ApplicationsController } from "../../state/hooks/applications/useApplications";
import {
  AlertCircle,
  Check,
  Eye,
  Loader,
  Play,
  Plus,
  Server,
  Square,
  Trash,
  X,
} from "../primitives/icons";

export function ApplicationsSection({
  controller,
}: {
  controller: ApplicationsController;
}) {
  const { scope, catalog, instances } = controller;

  // Only images that support this scope are installable here.
  const installable = useMemo(
    () => catalog.filter((img) => img.scopes.includes(scope)),
    [catalog, scope]
  );

  // One instance per image per scope: mark already-installed images.
  const installedIds = useMemo(
    () => new Set(instances.map((i) => i.imageId)),
    [instances]
  );

  return (
    <div class="space-y-5">
      {controller.error && (
        <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
          <AlertCircle class="w-4 h-4 mt-0.5 flex-none text-accent-red" />
          <div class="text-accent-red break-words">{controller.error}</div>
        </div>
      )}

      <InstalledList controller={controller} />

      <div class="space-y-2.5">
        <h3 class="text-[13px] font-medium text-ink-100">Available applications</h3>
        <CatalogGrid images={installable} installedIds={installedIds} controller={controller} />
      </div>

      <p class="text-[11.5px] text-ink-400 leading-relaxed">
        {scope === "global"
          ? "Global applications run in their own container and are shared across the whole server."
          : "Project applications run inside this project's container. They are reachable on the host port below and, inside the project, on the LXD bridge."}{" "}
        Each app is exposed on a host port chosen to avoid conflicts (e.g.
        PostgreSQL <span class="font-mono">5432</span> inside →{" "}
        <span class="font-mono">5433</span> outside).
      </p>
    </div>
  );
}

// ---- catalog ---------------------------------------------------------------

function CatalogGrid({
  images,
  installedIds,
  controller,
}: {
  images: AppImage[];
  installedIds: Set<string>;
  controller: ApplicationsController;
}) {
  const [installing, setInstalling] = useState<AppImage | null>(null);

  if (controller.catalogLoading && images.length === 0) {
    return <Muted text="Loading catalog…" />;
  }
  if (images.length === 0) {
    return <Muted text="No applications available for this scope." />;
  }
  return (
    <>
      <div class="grid gap-2.5 sm:grid-cols-2">
        {images.map((img) => (
          <CatalogCard
            key={img.id}
            image={img}
            installed={installedIds.has(img.id)}
            onInstall={() => setInstalling(img)}
          />
        ))}
      </div>
      {installing && (
        <InstallDialog
          image={installing}
          onClose={() => setInstalling(null)}
          onInstall={controller.install}
        />
      )}
    </>
  );
}

function CatalogCard({
  image,
  installed,
  onInstall,
}: {
  image: AppImage;
  installed: boolean;
  onInstall: () => void;
}) {
  return (
    <div class="rounded-md border border-white/[0.08] bg-white/[0.03] p-3 flex items-start gap-3">
      <div class="h-9 w-9 flex-none rounded-md bg-white/[0.06] grid place-items-center text-ink-200">
        <Server class="w-4 h-4" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <span class="text-[13.5px] font-medium text-ink-50 truncate">{image.name}</span>
          {image.version && (
            <span class="text-[10.5px] text-ink-400 font-mono">{image.version}</span>
          )}
        </div>
        {image.description && (
          <p class="mt-0.5 text-[12px] text-ink-300 leading-snug line-clamp-2">
            {image.description}
          </p>
        )}
      </div>
      {installed ? (
        <span
          title="Already installed in this scope"
          class="h-8 px-2.5 flex-none rounded border border-white/10 text-ink-400 text-[12px] font-medium inline-flex items-center gap-1"
        >
          <Check class="w-3.5 h-3.5" />
          Installed
        </span>
      ) : (
        <button
          type="button"
          onClick={onInstall}
          class="h-8 px-2.5 flex-none rounded bg-accent-blue/80 hover:bg-accent-blue text-white text-[12px] font-medium inline-flex items-center gap-1"
        >
          <Plus class="w-3.5 h-3.5" />
          Install
        </button>
      )}
    </div>
  );
}

function InstallDialog({
  image,
  onClose,
  onInstall,
}: {
  image: AppImage;
  onClose: () => void;
  onInstall: ApplicationsController["install"];
}) {
  const [name, setName] = useState(image.name);
  const [env, setEnv] = useState<Record<string, string>>({});
  const [externalPort, setExternalPort] = useState<string>("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async (event: Event) => {
    event.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      const port = externalPort.trim() ? Number(externalPort.trim()) : undefined;
      if (port !== undefined && (!Number.isInteger(port) || port < 1 || port > 65535)) {
        throw new Error("External port must be 1–65535.");
      }
      await onInstall({
        imageId: image.id,
        name: name.trim() || image.name,
        env,
        externalPort: port,
      });
      onClose();
    } catch (error) {
      setErr((error as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      class="fixed inset-0 z-50 grid place-items-center bg-black/60 p-4"
      onClick={onClose}
    >
      <form
        onSubmit={submit}
        onClick={(e) => e.stopPropagation()}
        class="w-full max-w-md rounded-lg border border-white/10 bg-[#0f1217] p-4 space-y-3 shadow-xl"
      >
        <div class="flex items-center gap-2">
          <Server class="w-4 h-4 text-ink-200" />
          <h3 class="text-[14px] font-semibold text-ink-50">Install {image.name}</h3>
          <button
            type="button"
            onClick={onClose}
            class="ml-auto h-7 w-7 rounded text-ink-300 hover:text-ink-100 hover:bg-white/[0.08] grid place-items-center"
            aria-label="Close"
          >
            <X class="w-4 h-4" />
          </button>
        </div>

        <label class="block space-y-1">
          <span class="text-[11.5px] text-ink-300">Display name</span>
          <input
            value={name}
            onInput={(e) => setName((e.target as HTMLInputElement).value)}
            class="w-full h-9 px-2.5 rounded border border-white/10 bg-black/30 text-[13px] text-ink-50 focus:outline-none focus:border-accent-blue/50"
          />
        </label>

        {(image.env ?? []).map((v) => (
          <label key={v.key} class="block space-y-1">
            <span class="text-[11.5px] text-ink-300">
              {v.label || v.key}
              {v.required && <span class="text-accent-red"> *</span>}
              {v.generate === "password" && (
                <span class="text-ink-400"> — leave blank to auto-generate</span>
              )}
            </span>
            <input
              type={v.secret ? "password" : "text"}
              value={env[v.key] ?? ""}
              placeholder={v.default ? `default: ${v.default}` : ""}
              autoComplete="off"
              spellcheck={false}
              onInput={(e) =>
                setEnv((cur) => ({ ...cur, [v.key]: (e.target as HTMLInputElement).value }))
              }
              class="w-full h-9 px-2.5 rounded border border-white/10 bg-black/30 text-[13px] font-mono text-ink-50 placeholder-ink-400 focus:outline-none focus:border-accent-blue/50"
            />
          </label>
        ))}

        <label class="block space-y-1">
          <span class="text-[11.5px] text-ink-300">
            Host port <span class="text-ink-400">— blank auto-picks a free one (internal {image.port.internal})</span>
          </span>
          <input
            value={externalPort}
            inputMode="numeric"
            placeholder={String(image.port.defaultExternal || image.port.internal)}
            onInput={(e) => setExternalPort((e.target as HTMLInputElement).value)}
            class="w-full h-9 px-2.5 rounded border border-white/10 bg-black/30 text-[13px] font-mono text-ink-50 placeholder-ink-400 focus:outline-none focus:border-accent-blue/50"
          />
        </label>

        {err && <div class="text-[12px] text-accent-red">{err}</div>}

        <div class="flex justify-end gap-2 pt-1">
          <button
            type="button"
            onClick={onClose}
            class="h-9 px-3 rounded text-[13px] text-ink-300 hover:text-ink-100 hover:bg-white/[0.08]"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={busy}
            class="h-9 px-3.5 rounded bg-accent-blue/80 hover:bg-accent-blue text-white text-[13px] font-medium disabled:opacity-50 inline-flex items-center gap-1.5"
          >
            {busy && <Loader class="w-3.5 h-3.5 animate-spin" />}
            {busy ? "Installing…" : "Install"}
          </button>
        </div>
      </form>
    </div>
  );
}

// ---- installed list --------------------------------------------------------

function InstalledList({ controller }: { controller: ApplicationsController }) {
  const { instances, loading } = controller;
  if (loading && instances.length === 0) return <Muted text="Loading applications…" />;
  if (instances.length === 0) {
    return <Muted text="No applications installed yet." />;
  }
  return (
    <div class="space-y-2">
      {instances.map((inst) => (
        <InstalledRow key={inst.id} inst={inst} controller={controller} />
      ))}
    </div>
  );
}

function InstalledRow({
  inst,
  controller,
}: {
  inst: AppInstance;
  controller: ApplicationsController;
}) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [editingPort, setEditingPort] = useState(false);
  const [portDraft, setPortDraft] = useState(String(inst.externalPort));

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    setErr(null);
    try {
      await fn();
    } catch (error) {
      setErr((error as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const savePort = () =>
    run(async () => {
      const port = Number(portDraft.trim());
      if (!Number.isInteger(port) || port < 1 || port > 65535) {
        throw new Error("Port must be 1–65535.");
      }
      await controller.setPort(inst.id, port);
      setEditingPort(false);
    });

  const remove = () => {
    if (!confirm(`Uninstall ${inst.name}? This removes its host port mapping.`)) return;
    void run(() => controller.uninstall(inst.id));
  };

  const running = inst.status === "running";

  return (
    <div class="rounded-md border border-white/[0.08] bg-white/[0.03] px-3 py-2.5 space-y-1.5">
      <div class="flex items-center gap-2 min-w-0">
        <Server class="w-4 h-4 flex-none text-ink-300" />
        <span class="text-[13px] font-medium text-ink-50 truncate">{inst.name}</span>
        <span class="text-[11px] text-ink-400 font-mono">{inst.imageId}</span>
        <StatusBadge status={inst.status} />
        <div class="ml-auto flex items-center gap-1">
          {running ? (
            <IconBtn title="Stop" onClick={() => void run(() => controller.stop(inst.id))} disabled={busy}>
              <Square class="w-3.5 h-3.5" />
            </IconBtn>
          ) : (
            <IconBtn title="Start" onClick={() => void run(() => controller.start(inst.id))} disabled={busy}>
              <Play class="w-3.5 h-3.5" />
            </IconBtn>
          )}
          <IconBtn title="Uninstall" onClick={remove} disabled={busy} danger>
            <Trash class="w-3.5 h-3.5" />
          </IconBtn>
        </div>
      </div>

      <div class="flex items-center gap-2 text-[12px] text-ink-300 flex-wrap">
        <span class="text-ink-400">host</span>
        {editingPort ? (
          <span class="inline-flex items-center gap-1">
            <input
              value={portDraft}
              inputMode="numeric"
              onInput={(e) => setPortDraft((e.target as HTMLInputElement).value)}
              class="h-7 w-20 px-2 rounded border border-white/10 bg-black/30 text-[12px] font-mono text-ink-50 focus:outline-none focus:border-accent-blue/50"
            />
            <IconBtn title="Save port" onClick={savePort} disabled={busy}>
              <Check class="w-3.5 h-3.5" />
            </IconBtn>
            <IconBtn
              title="Cancel"
              onClick={() => {
                setEditingPort(false);
                setPortDraft(String(inst.externalPort));
              }}
              disabled={busy}
            >
              <X class="w-3.5 h-3.5" />
            </IconBtn>
          </span>
        ) : (
          <button
            type="button"
            onClick={() => setEditingPort(true)}
            title="Change host port"
            class="font-mono text-ink-100 hover:text-accent-blue underline decoration-dotted underline-offset-2"
          >
            {inst.bindAddress}:{inst.externalPort}
          </button>
        )}
        <span class="text-ink-400">→ container</span>
        <span class="font-mono text-ink-100">{inst.internalPort}</span>
        {inst.envPublic &&
          Object.entries(inst.envPublic).map(([k, v]) => (
            <span key={k} class="text-ink-400 font-mono">
              · {k}=<span class="text-ink-200">{v}</span>
            </span>
          ))}
      </div>

      <ConnectionDetails inst={inst} controller={controller} />

      {inst.error && inst.status === "error" && (
        <div class="text-[11.5px] text-accent-red break-words">{inst.error}</div>
      )}
      {err && <div class="text-[11.5px] text-accent-red break-words">{err}</div>}
    </div>
  );
}

// ConnectionDetails shows how to reach an installed app and, on demand, its
// (otherwise redacted) credentials fetched from the authorized endpoint.
function ConnectionDetails({
  inst,
  controller,
}: {
  inst: AppInstance;
  controller: ApplicationsController;
}) {
  const [open, setOpen] = useState(false);
  const [creds, setCreds] = useState<AppCredentials | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const toggle = async () => {
    if (open) {
      setOpen(false);
      return;
    }
    setOpen(true);
    if (creds) return;
    setLoading(true);
    setErr(null);
    try {
      setCreds(await controller.credentials(inst.id));
    } catch (error) {
      setErr((error as Error).message);
    } finally {
      setLoading(false);
    }
  };

  const fromContainers = `${inst.containerName}.lxd:${inst.internalPort}`;
  const fromHost = `${inst.bindAddress}:${inst.externalPort}`;

  return (
    <div class="space-y-1.5">
      <button
        type="button"
        onClick={toggle}
        class="text-[11.5px] text-ink-300 hover:text-accent-blue inline-flex items-center gap-1"
      >
        <Eye class="w-3.5 h-3.5" />
        {open ? "Hide connection" : "Connection & credentials"}
      </button>

      {open && (
        <div class="rounded-md border border-white/[0.08] bg-black/20 p-2.5 space-y-1.5 text-[12px]">
          <CopyLine label="Host (containers)" value={fromContainers} />
          <CopyLine label="Host (host)" value={fromHost} />
          <CopyLine label="Port (internal)" value={String(inst.internalPort)} />
          <CopyLine label="Port (host)" value={String(inst.externalPort)} />

          {loading && <div class="text-ink-400">Loading credentials…</div>}
          {err && <div class="text-accent-red break-words">{err}</div>}

          {creds && (
            <>
              {creds.username && <CopyLine label="User" value={creds.username} />}
              {creds.password && <SecretLine label="Password" value={creds.password} />}
              {creds.database && <CopyLine label="Database" value={creds.database} />}
              {!creds.username && !creds.password && !creds.database && (
                <div class="text-ink-400">No credentials required.</div>
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}

function CopyLine({ label, value }: { label: string; value: string }) {
  return (
    <div class="flex items-center gap-2">
      <span class="text-ink-400 w-40">{label}</span>
      <code class="flex-1 font-mono text-ink-100 break-all">{value}</code>
      <button
        type="button"
        onClick={() => void navigator.clipboard?.writeText(value)}
        class="text-[11px] text-ink-300 hover:text-ink-100"
      >
        copy
      </button>
    </div>
  );
}

function SecretLine({ label, value }: { label: string; value: string }) {
  const [shown, setShown] = useState(false);
  return (
    <div class="flex items-center gap-2">
      <span class="text-ink-400 w-40">{label}</span>
      <code class="flex-1 font-mono text-ink-100 break-all">
        {shown ? value : "•".repeat(Math.min(16, value.length || 8))}
      </code>
      <button
        type="button"
        onClick={() => setShown((v) => !v)}
        class="text-[11px] text-ink-300 hover:text-ink-100"
      >
        {shown ? "hide" : "show"}
      </button>
      <button
        type="button"
        onClick={() => void navigator.clipboard?.writeText(value)}
        class="text-[11px] text-ink-300 hover:text-ink-100"
      >
        copy
      </button>
    </div>
  );
}

// ---- small helpers ---------------------------------------------------------

function StatusBadge({ status }: { status: AppInstanceStatus }) {
  const map: Record<AppInstanceStatus, string> = {
    running: "text-accent-green border-accent-green/30 bg-accent-green/[0.08]",
    stopped: "text-ink-300 border-white/15 bg-white/[0.04]",
    installing: "text-accent-blue border-accent-blue/30 bg-accent-blue/[0.08]",
    error: "text-accent-red border-accent-red/30 bg-accent-red/[0.08]",
  };
  return (
    <span class={`text-[10.5px] px-1.5 py-0.5 rounded border ${map[status]}`}>
      {status}
    </span>
  );
}

function IconBtn({
  children,
  title,
  onClick,
  disabled,
  danger,
}: {
  children: ComponentChildren;
  title: string;
  onClick: () => void;
  disabled?: boolean;
  danger?: boolean;
}) {
  return (
    <button
      type="button"
      title={title}
      aria-label={title}
      onClick={onClick}
      disabled={disabled}
      class={`h-7 w-7 rounded grid place-items-center text-ink-300 hover:bg-white/[0.08] disabled:opacity-40 ${
        danger ? "hover:text-accent-red" : "hover:text-ink-50"
      }`}
    >
      {children}
    </button>
  );
}

function Muted({ text }: { text: string }) {
  return (
    <div class="rounded-md border border-dashed border-white/10 px-3 py-4 text-center text-[12.5px] text-ink-400">
      {text}
    </div>
  );
}

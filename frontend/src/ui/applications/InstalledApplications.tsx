import type { ComponentChildren } from "preact";
import { useMemo, useState } from "preact/hooks";
import type {
  AppCredentials,
  AppImage,
  AppInstance,
  AppInstanceStatus,
} from "../../models/application";
import { EXTENSION_SLOTS } from "../../config/extensions";
import { useConfirm } from "../../state/context/ConfirmContext";
import type { ApplicationsController } from "../../state/hooks/applications/useApplications";
import { ExtensionSlot } from "../primitives/ExtensionSlot";
import {
  Check,
  Eye,
  Play,
  Square,
  Trash,
  X,
} from "../primitives/icons";
import { AppIcon } from "./AppIcon";
import { ApplicationEmptyState } from "./ApplicationEmptyState";
import {
  extensionSummary,
  hasContainer,
  uninstallConsequence,
} from "./applicationPresentation";

export function InstalledList({ controller }: { controller: ApplicationsController }) {
  const { instances, loading } = controller;
  const imagesById = useMemo(
    () => new Map(controller.catalog.map((image) => [image.id, image])),
    [controller.catalog],
  );
  if (loading && instances.length === 0) {
    return <ApplicationEmptyState text="Loading applications…" />;
  }
  if (instances.length === 0) {
    return <ApplicationEmptyState text="No applications installed yet." />;
  }
  return (
    <div class="space-y-2">
      {instances.map((instance) => (
        <InstalledRow
          key={instance.id}
          instance={instance}
          image={imagesById.get(instance.imageId)}
          controller={controller}
        />
      ))}
    </div>
  );
}

function InstalledRow({
  instance,
  image,
  controller,
}: {
  instance: AppInstance;
  /** Catalog entry, if the catalog has loaded; drives the icon and layout. */
  image?: AppImage;
  controller: ApplicationsController;
}) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [editingPort, setEditingPort] = useState(false);
  const [portDraft, setPortDraft] = useState(String(instance.externalPort));
  const confirm = useConfirm();

  const run = async (action: () => Promise<void>) => {
    setBusy(true);
    setErr(null);
    try {
      await action();
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
      await controller.setPort(instance.id, port);
      setEditingPort(false);
    });

  const running = instance.status === "running";
  // A UI or backend image has no container, port, or credentials — only an
  // extension that is on or off. Showing it a port row would be showing it
  // zeros.
  const imageHasContainer = hasContainer(image);

  const remove = async () => {
    // The dialog owns the request: a failure is shown inside it so the user can
    // retry or back out, instead of closing and leaving an error behind a row.
    await confirm({
      title: `Uninstall ${instance.name}?`,
      description: imageHasContainer ? "This cannot be undone." : undefined,
      message: uninstallConsequence(instance, imageHasContainer),
      confirmLabel: "Uninstall",
      pendingLabel: "Uninstalling…",
      tone: "danger",
      action: () => controller.uninstall(instance.id),
    });
  };

  return (
    <div class="rounded-md border border-white/[0.08] bg-white/[0.03] px-3 py-2.5 space-y-1.5">
      <div class="flex items-center gap-2 min-w-0">
        <span class="flex-none text-ink-300">
          <AppIcon
            image={image ?? { id: instance.imageId, name: instance.name }}
            class="w-4 h-4"
          />
        </span>
        <span class="text-[13px] font-medium text-ink-50 truncate">{instance.name}</span>
        <span class="text-[11px] text-ink-400 font-mono">{instance.imageId}</span>
        <StatusBadge status={instance.status} />
        <div class="ml-auto flex items-center gap-1">
          <ExtensionSlot
            name={EXTENSION_SLOTS.applicationCardActions}
            scope={controller.scope}
            projectId={instance.projectId}
            instance={instance}
          />
          {running ? (
            <IconButton
              title="Stop"
              onClick={() => void run(() => controller.stop(instance.id))}
              disabled={busy}
            >
              <Square class="w-3.5 h-3.5" />
            </IconButton>
          ) : (
            <IconButton
              title="Start"
              onClick={() => void run(() => controller.start(instance.id))}
              disabled={busy}
            >
              <Play class="w-3.5 h-3.5" />
            </IconButton>
          )}
          <IconButton
            title="Uninstall"
            onClick={() => void remove()}
            disabled={busy}
            danger
          >
            <Trash class="w-3.5 h-3.5" />
          </IconButton>
        </div>
      </div>

      {imageHasContainer ? (
        <div class="flex items-center gap-2 text-[12px] text-ink-300 flex-wrap">
          <span class="text-ink-400">host</span>
          {editingPort ? (
            <span class="inline-flex items-center gap-1">
              <input
                value={portDraft}
                inputMode="numeric"
                onInput={(event) => setPortDraft((event.target as HTMLInputElement).value)}
                class="h-7 w-20 px-2 rounded border border-white/10 bg-black/30 text-[12px] font-mono text-ink-50 focus:outline-none focus:border-accent-blue/50"
              />
              <IconButton title="Save port" onClick={savePort} disabled={busy}>
                <Check class="w-3.5 h-3.5" />
              </IconButton>
              <IconButton
                title="Cancel"
                onClick={() => {
                  setEditingPort(false);
                  setPortDraft(String(instance.externalPort));
                }}
                disabled={busy}
              >
                <X class="w-3.5 h-3.5" />
              </IconButton>
            </span>
          ) : (
            <button
              type="button"
              onClick={() => setEditingPort(true)}
              title="Change host port"
              class="font-mono text-ink-100 hover:text-accent-blue underline decoration-dotted underline-offset-2"
            >
              {instance.bindAddress}:{instance.externalPort}
            </button>
          )}
          <span class="text-ink-400">→ container</span>
          <span class="font-mono text-ink-100">{instance.internalPort}</span>
          {instance.envPublic &&
            Object.entries(instance.envPublic).map(([key, value]) => (
              <span key={key} class="text-ink-400 font-mono">
                · {key}=<span class="text-ink-200">{value}</span>
              </span>
            ))}
        </div>
      ) : (
        <div class="text-[12px] text-ink-400">
          {extensionSummary(image, running)}
        </div>
      )}

      {imageHasContainer && (
        <ConnectionDetails instance={instance} controller={controller} />
      )}

      {instance.error && instance.status === "error" && (
        <div class="text-[11.5px] text-accent-red break-words">{instance.error}</div>
      )}
      {err && <div class="text-[11.5px] text-accent-red break-words">{err}</div>}
    </div>
  );
}

// ConnectionDetails shows how to reach an installed app and, on demand, its
// (otherwise redacted) credentials fetched from the authorized endpoint.
function ConnectionDetails({
  instance,
  controller,
}: {
  instance: AppInstance;
  controller: ApplicationsController;
}) {
  const [open, setOpen] = useState(false);
  const [credentials, setCredentials] = useState<AppCredentials | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const toggle = async () => {
    if (open) {
      setOpen(false);
      return;
    }
    setOpen(true);
    if (credentials) return;
    setLoading(true);
    setErr(null);
    try {
      setCredentials(await controller.credentials(instance.id));
    } catch (error) {
      setErr((error as Error).message);
    } finally {
      setLoading(false);
    }
  };

  const fromContainers = `${instance.containerName}.lxd:${instance.internalPort}`;
  const fromHost = `${instance.bindAddress}:${instance.externalPort}`;

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
          <CopyLine label="Port (internal)" value={String(instance.internalPort)} />
          <CopyLine label="Port (host)" value={String(instance.externalPort)} />

          {loading && <div class="text-ink-400">Loading credentials…</div>}
          {err && <div class="text-accent-red break-words">{err}</div>}

          {credentials && (
            <>
              {credentials.username && <CopyLine label="User" value={credentials.username} />}
              {credentials.password && (
                <SecretLine label="Password" value={credentials.password} />
              )}
              {credentials.database && (
                <CopyLine label="Database" value={credentials.database} />
              )}
              {!credentials.username && !credentials.password && !credentials.database && (
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
        onClick={() => setShown((current) => !current)}
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

function StatusBadge({ status }: { status: AppInstanceStatus }) {
  const styles: Record<AppInstanceStatus, string> = {
    running: "text-accent-green border-accent-green/30 bg-accent-green/[0.08]",
    stopped: "text-ink-300 border-white/15 bg-white/[0.04]",
    installing: "text-accent-blue border-accent-blue/30 bg-accent-blue/[0.08]",
    error: "text-accent-red border-accent-red/30 bg-accent-red/[0.08]",
  };
  return (
    <span class={`text-[10.5px] px-1.5 py-0.5 rounded border ${styles[status]}`}>
      {status}
    </span>
  );
}

function IconButton({
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

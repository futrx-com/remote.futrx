import { useState } from "preact/hooks";
import type { AppImage } from "../../models/application";
import type { ApplicationsController } from "../../state/hooks/applications/useApplications";
import { AppIcon } from "./AppIcon";
import { ApplicationEmptyState } from "./ApplicationEmptyState";
import {
  Check,
  Loader,
  Plus,
  RotateCcw,
  Server,
  X,
} from "../primitives/icons";

export function CatalogGrid({
  images,
  installedIds,
  failedIds,
  controller,
}: {
  images: AppImage[];
  installedIds: Set<string>;
  failedIds: Set<string>;
  controller: ApplicationsController;
}) {
  const [installing, setInstalling] = useState<AppImage | null>(null);

  if (controller.catalogLoading && images.length === 0) {
    return <ApplicationEmptyState text="Loading catalog…" />;
  }
  if (images.length === 0) {
    return <ApplicationEmptyState text="No applications available for this scope." />;
  }
  return (
    <>
      <div class="grid gap-2.5 sm:grid-cols-2">
        {images.map((image) => (
          <CatalogCard
            key={image.id}
            image={image}
            installed={installedIds.has(image.id)}
            failed={failedIds.has(image.id)}
            onInstall={() => setInstalling(image)}
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
  failed,
  onInstall,
}: {
  image: AppImage;
  installed: boolean;
  failed: boolean;
  onInstall: () => void;
}) {
  return (
    <div class="rounded-md border border-white/[0.08] bg-white/[0.03] p-3 flex items-start gap-3">
      <div class="h-9 w-9 flex-none rounded-md bg-white/[0.06] grid place-items-center text-ink-200">
        <AppIcon image={image} class="w-4 h-4" />
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
          title={failed
            ? "The last install failed. Retrying removes that attempt and installs again."
            : undefined}
          class="h-8 px-2.5 flex-none rounded bg-accent-blue/80 hover:bg-accent-blue text-white text-[12px] font-medium inline-flex items-center gap-1"
        >
          {failed ? <RotateCcw class="w-3.5 h-3.5" /> : <Plus class="w-3.5 h-3.5" />}
          {failed ? "Retry" : "Install"}
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
        onClick={(event) => event.stopPropagation()}
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
            onInput={(event) => setName((event.target as HTMLInputElement).value)}
            class="w-full h-9 px-2.5 rounded border border-white/10 bg-black/30 text-[13px] text-ink-50 focus:outline-none focus:border-accent-blue/50"
          />
        </label>

        {(image.env ?? []).map((variable) => (
          <label key={variable.key} class="block space-y-1">
            <span class="text-[11.5px] text-ink-300">
              {variable.label || variable.key}
              {variable.required && <span class="text-accent-red"> *</span>}
              {variable.generate === "password" && (
                <span class="text-ink-400"> — leave blank to auto-generate</span>
              )}
            </span>
            <input
              type={variable.secret ? "password" : "text"}
              value={env[variable.key] ?? ""}
              placeholder={variable.default ? `default: ${variable.default}` : ""}
              autoComplete="off"
              spellcheck={false}
              onInput={(event) =>
                setEnv((current) => ({
                  ...current,
                  [variable.key]: (event.target as HTMLInputElement).value,
                }))
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
            onInput={(event) => setExternalPort((event.target as HTMLInputElement).value)}
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

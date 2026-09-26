import { useState } from "preact/hooks";
import type { AppApplication } from "../../models/application";
import type { ApplicationsController } from "../../state/hooks/applications/useApplications";
import { prepareInstallRequest } from "../../services/applications/prepareInstallRequest";
import { AppIcon } from "./AppIcon";
import { ApplicationEmptyState } from "./ApplicationEmptyState";
import { hasPortBinding } from "./applicationPresentation";
import {
  Check,
  Loader,
  Plus,
  RotateCcw,
  Server,
  X,
} from "../primitives/icons";

export function CatalogGrid({
  applications,
  installedIds,
  failedIds,
  controller,
}: {
  applications: AppApplication[];
  installedIds: Set<string>;
  failedIds: Set<string>;
  controller: ApplicationsController;
}) {
  const [installing, setInstalling] = useState<AppApplication | null>(null);

  if (controller.catalogLoading && applications.length === 0) {
    return <ApplicationEmptyState text="Loading catalog…" />;
  }
  if (applications.length === 0) {
    return <ApplicationEmptyState text="No applications available for this scope." />;
  }
  return (
    <>
      <div class="grid gap-2.5 sm:grid-cols-2">
        {applications.map((application) => (
          <CatalogCard
            key={application.id}
            application={application}
            installed={installedIds.has(application.id)}
            failed={failedIds.has(application.id)}
            onInstall={() => setInstalling(application)}
          />
        ))}
      </div>
      {installing && (
        <InstallDialog
          application={installing}
          onClose={() => setInstalling(null)}
          onInstall={controller.install}
        />
      )}
    </>
  );
}

function CatalogCard({
  application,
  installed,
  failed,
  onInstall,
}: {
  application: AppApplication;
  installed: boolean;
  failed: boolean;
  onInstall: () => void;
}) {
  return (
    <div class="rounded-md border border-white/[0.08] bg-white/[0.03] p-3 flex items-start gap-3">
      <div class="h-9 w-9 flex-none rounded-md bg-white/[0.06] grid place-items-center text-ink-200">
        <AppIcon application={application} class="w-4 h-4" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <span class="text-[13.5px] font-medium text-ink-50 truncate">{application.name}</span>
          {application.version && (
            <span class="text-[10.5px] text-ink-400 font-mono">{application.version}</span>
          )}
          {/* An uploaded app installs exactly like a built-in one; the badge
              says where it came from, because that is what tells an admin it
              can be removed from this server. */}
          {application.source === "uploaded" && (
            <span
              title="Uploaded to this server"
              class="text-[10px] px-1.5 h-4 rounded inline-flex items-center text-ink-300 bg-white/[0.08]"
            >
              uploaded
            </span>
          )}
        </div>
        {application.description && (
          <p class="mt-0.5 text-[12px] text-ink-300 leading-snug line-clamp-2">
            {application.description}
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
  application,
  onClose,
  onInstall,
}: {
  application: AppApplication;
  onClose: () => void;
  onInstall: ApplicationsController["install"];
}) {
  const [name, setName] = useState(application.name);
  const [env, setEnv] = useState<Record<string, string>>({});
  const [externalPort, setExternalPort] = useState<string>("");
  // An application without port-bearing infrastructure has no host port to configure.
  const asksForPort = hasPortBinding(application);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async (event: Event) => {
    event.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      await onInstall(prepareInstallRequest(application, { name, env, externalPort }, asksForPort));
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
      {/* The field list is as long as the application's env[] makes it, so the
          surface is capped to the viewport and the fields scroll inside it.
          The title and the buttons stay put: an install that cannot be
          confirmed because its button is below the fold is not installable. */}
      <form
        onSubmit={submit}
        onClick={(event) => event.stopPropagation()}
        class={`${application.env?.some((variable) => variable.format === "json") ? "max-w-2xl" : "max-w-md"} w-full max-h-[calc(100dvh-2rem)] flex flex-col rounded-lg border border-white/10 bg-[#0f1217] shadow-xl`}
      >
        <div class="flex-none flex items-center gap-2 px-4 pt-4 pb-3">
          <Server class="w-4 h-4 text-ink-200" />
          <h3 class="text-[14px] font-semibold text-ink-50">Install {application.name}</h3>
          <button
            type="button"
            onClick={onClose}
            class="ml-auto h-7 w-7 rounded text-ink-300 hover:text-ink-100 hover:bg-white/[0.08] grid place-items-center"
            aria-label="Close"
          >
            <X class="w-4 h-4" />
          </button>
        </div>

        <div class="flex-1 min-h-0 overflow-y-auto touch-scroll px-4 pb-3 space-y-3">
          <label class="block space-y-1">
            <span class="text-[11.5px] text-ink-300">Display name</span>
            <input
              value={name}
              onInput={(event) => setName((event.target as HTMLInputElement).value)}
              class="w-full h-9 px-2.5 rounded border border-white/10 bg-black/30 text-[13px] text-ink-50 focus:outline-none focus:border-accent-blue/50"
            />
          </label>

          {(application.env ?? []).map((variable) => (
            <label key={variable.key} class="block space-y-1">
              <span class="text-[11.5px] text-ink-300">
                {variable.label || variable.key}
                {variable.required && <span class="text-accent-red"> *</span>}
                {variable.generate === "password" && (
                  <span class="text-ink-400"> — leave blank to auto-generate</span>
                )}
              </span>
              {variable.format === "json" ? (
                <>
                  <textarea
                    value={env[variable.key] ?? variable.default ?? ""}
                    rows={16}
                    spellcheck={false}
                    onInput={(event) => setEnv((current) => ({
                      ...current,
                      [variable.key]: (event.target as HTMLTextAreaElement).value,
                    }))}
                    class="w-full min-h-48 px-2.5 py-2 rounded border border-white/10 bg-black/30 text-[12px] leading-relaxed font-mono text-ink-50 focus:outline-none focus:border-accent-blue/50 resize-y"
                  />
                  <span class="block text-[11px] text-ink-400">
                    Edit the complete JSON object before installing. The saved value is hidden from installed app details.
                  </span>
                </>
              ) : (
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
              )}
            </label>
          ))}

          {asksForPort && (
            <label class="block space-y-1">
              <span class="text-[11.5px] text-ink-300">
                Host port <span class="text-ink-400">— blank auto-picks a free one (internal {application.port.internal})</span>
              </span>
              <input
                value={externalPort}
                inputMode="numeric"
                placeholder={String(application.port.defaultExternal || application.port.internal)}
                onInput={(event) => setExternalPort((event.target as HTMLInputElement).value)}
                class="w-full h-9 px-2.5 rounded border border-white/10 bg-black/30 text-[13px] font-mono text-ink-50 placeholder-ink-400 focus:outline-none focus:border-accent-blue/50"
              />
            </label>
          )}
        </div>

        <div class="flex-none px-4 pt-3 pb-4 space-y-2 border-t border-white/[0.06]">
          {/* A failed install carries the tail of the script's output, which
              runs to thousands of characters. It gets its own bounded, scrolling
              box: left to grow it squeezes the fields above it to nothing, and
              the reason for the failure is at the *end* of that output, so the
              box starts scrolled there. */}
          {err && (
            <div
              ref={(node) => {
                if (node) node.scrollTop = node.scrollHeight;
              }}
              class="max-h-24 overflow-y-auto touch-scroll rounded border border-accent-red/25 bg-accent-red/[0.06] px-2 py-1.5 text-[11.5px] leading-relaxed font-mono text-accent-red whitespace-pre-wrap break-words"
            >
              {err}
            </div>
          )}

          <div class="flex justify-end gap-2">
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
        </div>
      </form>
    </div>
  );
}

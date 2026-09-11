import { useRef, useState } from "preact/hooks";
import type { AppPackage, AppScope } from "../../models/application";
import { useConfirm } from "../../state/context/ConfirmContext";
import {
  describeInstalls,
  describeOutcome,
  packageScopes,
  packageSummary,
  whereToInstall,
} from "./applicationPresentation";
import type { ApplicationsController } from "../../state/hooks/applications/useApplications";
import {
  AlertCircle,
  Boxes,
  Loader,
  Trash,
  Upload,
} from "../primitives/icons";

/**
 * Uploading an application to this server's catalog.
 *
 * An application package is a .zip of exactly what a built-in application's
 * directory holds — `image.json`, an optional `install.sh`, an optional `ui/`,
 * an optional `plugin/`. Once uploaded it is an ordinary catalog entry: it
 * appears under "Available applications", installs the same way, and runs the
 * same way.
 *
 * It is stored in the server's state directory rather than in its binary,
 * which is what makes it survive an update: updating replaces the program and
 * its built-in catalog and leaves uploaded applications, their installed
 * instances and their settings exactly where they were.
 *
 * The same control appears in Settings and inside every project, because
 * uploading from a project is a convenience rather than a different act: there
 * is one catalog, and a package added from one project is added for all of
 * them. Only an administrator sees it — a package ships code that runs with the
 * server's privileges, so adding one is not a project-member power.
 */
export function ApplicationPackages({
  controller,
}: {
  controller: ApplicationsController;
}) {
  const confirm = useConfirm();
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [uploaded, setUploaded] = useState<AppPackage | null>(null);
  const [dragging, setDragging] = useState(false);

  const { packages, scope } = controller;

  const send = async (file: File) => {
    setBusy(true);
    setError(null);
    setUploaded(null);
    try {
      setUploaded(await controller.uploadPackage(file));
    } catch (uploadError) {
      setError((uploadError as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const pick = (event: Event) => {
    const target = event.target as HTMLInputElement;
    const file = target.files?.[0];
    // Clearing the input is what lets the same file be picked twice in a row,
    // which is exactly what re-uploading a fixed package looks like.
    target.value = "";
    if (file) void send(file);
  };

  const drop = (event: DragEvent) => {
    event.preventDefault();
    setDragging(false);
    if (busy) return;
    const file = event.dataTransfer?.files?.[0];
    if (file) void send(file);
  };

  const remove = async (pkg: AppPackage) => {
    const installs = pkg.installs ?? [];
    // Removing a package that is still installed has to uninstall those copies
    // first — otherwise they would point at a catalog entry that no longer
    // exists. That is real destruction, so the dialog names every copy it will
    // take down rather than asking the operator to go and find them.
    await confirm({
      title: `Remove ${pkg.name || pkg.id}?`,
      description:
        installs.length > 0
          ? `${installs.length} installed ${
              installs.length === 1 ? "copy" : "copies"
            } will be uninstalled first.`
          : "The uploaded package is deleted from this server.",
      message: (
        <>
          It disappears from the catalog for{" "}
          {scope === "project" ? "every project on this server" : "the whole server"}{" "}
          and can no longer be installed. Upload the .zip again to bring it back.
          {installs.length > 0 && (
            <>
              <span class="mt-2 block">Uninstalling first removes:</span>
              <ul class="mt-1 space-y-0.5">
                {installs.map((install) => (
                  <li key={install.instanceId} class="font-mono text-[11.5px]">
                    · {install.name}{" "}
                    {install.scope === "project"
                      ? `in project ${install.projectId}`
                      : "installed globally"}
                  </li>
                ))}
              </ul>
              <span class="mt-2 block">
                Anything those copies provisioned in a container — a database
                and its data included — goes with them.
              </span>
            </>
          )}
        </>
      ),
      confirmLabel: installs.length > 0 ? "Uninstall and remove" : "Remove",
      pendingLabel: "Removing…",
      tone: "danger",
      action: async () => {
        setError(null);
        await controller.removePackage(pkg.id, installs.length > 0);
      },
    });
  };

  return (
    <div class="space-y-2.5">
      <div class="space-y-0.5">
        <div class="flex items-center gap-2">
          <h3 class="text-[13px] font-medium text-ink-100">Uploaded apps in the catalog</h3>
          <span class="text-[11.5px] text-ink-400">
            {packages.length > 0
              ? `${packages.length} ${packages.length === 1 ? "package" : "packages"}`
              : "none yet"}
          </span>
        </div>
        {/* Uploading adds an app to one server-wide catalog. Where it can
            then be installed is the app's own decision, so the list says that
            per row rather than letting the page it sits on imply an answer. */}
        <p class="text-[11.5px] text-ink-400 leading-relaxed">
          {scope === "project"
            ? "Uploading adds an app to this server's catalog — every project can then install the ones that offer project scope. Admins only."
            : "These are available across the server. Each one installs only at the scopes it declares."}
        </p>
      </div>

      <div
        onDragOver={(event) => {
          event.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={drop}
        class={`rounded-md border border-dashed px-3 py-4 text-center transition-colors ${
          dragging
            ? "border-accent-blue/60 bg-accent-blue/[0.06]"
            : "border-white/[0.12] bg-white/[0.02]"
        }`}
      >
        <input
          ref={input}
          type="file"
          accept=".zip,application/zip,application/x-zip-compressed"
          onChange={pick}
          class="hidden"
        />
        <button
          type="button"
          disabled={busy}
          onClick={() => input.current?.click()}
          class="h-8 px-3 rounded bg-accent-blue/80 hover:bg-accent-blue text-white text-[12px] font-medium inline-flex items-center gap-1.5 disabled:opacity-50"
        >
          {busy ? <Loader class="w-3.5 h-3.5 animate-spin" /> : <Upload class="w-3.5 h-3.5" />}
          {busy ? "Uploading…" : "Upload app (.zip)"}
        </button>
        <p class="mt-2 text-[11.5px] text-ink-400 leading-relaxed">
          Drop a .zip here, or choose one. The archive holds the app's{" "}
          <span class="font-mono">image.json</span> at its root — or inside a
          single folder — alongside its{" "}
          <span class="font-mono">install.sh</span>,{" "}
          <span class="font-mono">ui/</span> and{" "}
          <span class="font-mono">plugin/</span>. Uploading again with the same
          id replaces that app — and if its{" "}
          <span class="font-mono">version</span> changed, every running copy is
          re-installed at the new one.
        </p>
      </div>

      {error && (
        <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[12px]">
          <AlertCircle class="w-4 h-4 mt-0.5 flex-none text-accent-red" />
          <div class="text-accent-red break-words whitespace-pre-wrap">{error}</div>
        </div>
      )}
      {uploaded && !error && <UploadReceipt pkg={uploaded} scope={scope} />}

      {packages.length > 0 && (
        <div class="space-y-1.5">
          {packages.map((pkg) => (
            <PackageRow
              key={pkg.id}
              pkg={pkg}
              scope={scope}
              onRemove={() => void remove(pkg)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

/**
 * What an upload actually did. A new version re-runs the install script inside
 * every container that already holds the app, so saying so — and naming any
 * copy it failed on — matters more than a bare "uploaded".
 */
function UploadReceipt({ pkg, scope }: { pkg: AppPackage; scope: AppScope }) {
  const upgraded = pkg.upgraded ?? [];
  const failed = upgraded.filter((outcome) => outcome.error);
  // Count what actually landed, not what was attempted: a receipt that claims
  // an upgrade the lines below it report as failed is worse than none.
  const succeeded = upgraded.length - failed.length;
  return (
    <div class="space-y-1">
      <p class="text-[11.5px] text-accent-green">
        {pkg.name || pkg.id} {pkg.version} is in the catalog.{" "}
        {upgraded.length === 0
          ? whereToInstall(pkg, scope)
          : succeeded === 0
            ? "No installed copy could be re-installed at the new version."
            : `Re-installed ${succeeded} ${
                succeeded === 1 ? "copy" : "copies"
              } at the new version.`}
      </p>
      {failed.map((outcome) => (
        <p
          key={outcome.instanceId}
          class="text-[11.5px] text-accent-red break-words whitespace-pre-wrap"
        >
          {describeOutcome(outcome)} could not be upgraded: {outcome.error}
        </p>
      ))}
    </div>
  );
}

function PackageRow({
  pkg,
  scope,
  onRemove,
}: {
  pkg: AppPackage;
  scope: AppScope;
  onRemove: () => void;
}) {
  return (
    <div class="rounded-md border border-white/[0.08] bg-white/[0.03] px-3 py-2 flex items-start gap-2.5">
      <Boxes class="w-4 h-4 mt-0.5 flex-none text-ink-300" />
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2 flex-wrap">
          <span class="text-[13px] text-ink-50 truncate">{pkg.name || pkg.id}</span>
          <span class="text-[10.5px] font-mono text-ink-400">{pkg.id}</span>
          {pkg.version && (
            <span class="text-[10.5px] font-mono text-ink-400">{pkg.version}</span>
          )}
        </div>
        <p class="mt-0.5 text-[11.5px] text-ink-400">{packageScopes(pkg, scope)}</p>
        {(pkg.installs?.length ?? 0) > 0 && (
          <p class="mt-0.5 text-[11.5px] text-ink-300">{describeInstalls(pkg)}</p>
        )}
        <p class="mt-0.5 text-[11.5px] text-ink-400">{packageSummary(pkg)}</p>
        {pkg.error && (
          <p class="mt-1 text-[11.5px] text-accent-red break-words whitespace-pre-wrap">
            Not in the catalog: {pkg.error}
          </p>
        )}
      </div>
      <button
        type="button"
        onClick={onRemove}
        title="Remove this uploaded application"
        aria-label={`Remove ${pkg.name || pkg.id}`}
        class="h-7 w-7 flex-none rounded text-ink-400 hover:text-accent-red hover:bg-white/[0.08] grid place-items-center"
      >
        <Trash class="w-3.5 h-3.5" />
      </button>
    </div>
  );
}

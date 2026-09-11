import { useMemo } from "preact/hooks";
import { EXTENSION_SLOTS } from "../../config/extensions";
import type { ApplicationsController } from "../../state/hooks/applications/useApplications";
import { ExtensionSlot } from "../primitives/ExtensionSlot";
import { AlertCircle } from "../primitives/icons";
import { CatalogGrid } from "./ApplicationCatalog";
import { InstalledList } from "./InstalledApplications";
import { catalogInstallationState } from "./applicationPresentation";

export function ApplicationsSection({
  controller,
}: {
  controller: ApplicationsController;
}) {
  const { scope, catalog, instances } = controller;

  // Only images that support this scope are installable here.
  const installable = useMemo(
    () => catalog.filter((image) => image.scopes.includes(scope)),
    [catalog, scope],
  );

  // One instance per image per scope. A failed install is not an installation:
  // it is an attempt that left an error to read, so its card offers a retry
  // rather than claiming the image is installed.
  const { installedImageIds, failedImageIds } = useMemo(
    () => catalogInstallationState(instances),
    [instances],
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
        <CatalogGrid
          images={installable}
          installedIds={installedImageIds}
          failedIds={failedImageIds}
          controller={controller}
        />
      </div>

      <ExtensionSlot
        name={EXTENSION_SLOTS.applicationsPanel}
        scope={scope}
        projectId={controller.projectId}
        class="block"
      />

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

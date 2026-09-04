import type {
  AppImage,
  AppInstance,
} from "../../models/application";

export interface CatalogInstallationState {
  installedImageIds: Set<string>;
  failedImageIds: Set<string>;
}

// Catalog cards distinguish completed installs from failed attempts: working
// instances disable installation, while failed ones offer the existing retry
// workflow. Keep both sets because persisted data may contain both states and
// the installed state must continue to win in the renderer.
export function catalogInstallationState(
  instances: AppInstance[],
): CatalogInstallationState {
  const installedImageIds = new Set<string>();
  const failedImageIds = new Set<string>();
  for (const instance of instances) {
    const target = instance.status === "error"
      ? failedImageIds
      : installedImageIds;
    target.add(instance.imageId);
  }
  return { installedImageIds, failedImageIds };
}

// Only service images expose container ports and credentials. An unknown
// catalog entry keeps the historical service presentation until the catalog
// finishes loading.
export function hasContainer(image: AppImage | undefined): boolean {
  return image?.type !== "ui" && image?.type !== "backend";
}

export function extensionSummary(
  image: AppImage | undefined,
  running: boolean,
): string {
  if (image?.type === "backend") {
    return running
      ? "Backend extension — a Go plugin runs on the server, not in a container."
      : "Backend extension — stopped. Start it to run its Go plugin.";
  }
  return running
    ? "Interface extension — nothing runs in a container. Its UI is loaded."
    : "Interface extension — nothing runs in a container. Start it to load its UI.";
}

export function uninstallConsequence(
  instance: AppInstance,
  imageHasContainer: boolean,
): string {
  if (!imageHasContainer) {
    return `“${instance.name}” stops contributing to the interface, and any plugin it runs is stopped and its data deleted. Nothing is removed from any container.`;
  }
  if (instance.scope === "global") {
    return `“${instance.name}” runs in its own container, which is deleted along with its data. The host port ${instance.bindAddress}:${instance.externalPort} is released.`;
  }
  return `“${instance.name}” is stopped and disabled, and the host port ${instance.bindAddress}:${instance.externalPort} is released. Installed packages and data stay in the project container.`;
}

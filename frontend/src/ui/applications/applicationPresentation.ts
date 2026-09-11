import type { AppImage, AppInstance } from "../../models/application";

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

// Whether installing this image put anything in a container. A tool did — it
// provisions software into the project's container — while a UI or backend
// image did not. This drives the uninstall wording, which has to say what is
// actually removed.
//
// The answer comes from the server, which owns the list of kinds: a kind added
// there would otherwise land here as whatever a local rule happened to say
// about a value it had never heard of. An image not in the catalog yet keeps
// the historical service presentation until the catalog finishes loading.
export function hasContainer(image: AppImage | undefined): boolean {
  return image?.needsContainer ?? true;
}

// Whether this image binds a host port, which is what the port row and the
// credentials panel are about. A tool runs in a container but exposes nothing,
// so showing it a port row would be showing it zeros.
export function hasPortBinding(image: AppImage | undefined): boolean {
  return image?.needsPort ?? true;
}

// The line shown in place of the port row for an image that has no port.
export function instanceSummary(
  image: AppImage | undefined,
  running: boolean,
): string {
  if (image?.type === "tool") {
    return running
      ? "Workspace tool — installed in this project's container. Nothing is exposed."
      : "Workspace tool — stopped. Start it to run it in the project's container.";
  }
  if (image?.type === "backend") {
    return running
      ? "Backend extension — a Go plugin runs on the server, not in a container."
      : "Backend extension — stopped. Start it to run its Go plugin.";
  }
  return running
    ? "Interface extension — nothing runs in a container. Its UI is loaded."
    : "Interface extension — nothing runs in a container. Start it to load its UI.";
}

/**
 * A stopped copy whose app has since moved to a new version. An upload
 * re-provisions the copies that are running and leaves stopped ones alone, so
 * this is the one state where the version an instance holds and the version
 * the catalog offers can drift — and starting it is what closes the gap.
 *
 * Only apps that reach a container can be stale: a UI or backend extension
 * provisions nothing, so a version bump changes nothing to re-run.
 */
export function pendingUpgradeVersion(
  instance: AppInstance,
  image: AppImage | undefined,
): string | null {
  if (!image || !hasContainer(image) || instance.status !== "stopped") return null;
  if (!image.version || image.version === instance.imageVersion) return null;
  return image.version;
}

export function uninstallConsequence(
  instance: AppInstance,
  image: AppImage | undefined,
): string {
  if (!hasContainer(image)) {
    return `“${instance.name}” stops contributing to the interface, and any plugin it runs is stopped and its data deleted. Nothing is removed from any container.`;
  }
  // A tool holds no host port, so there is none to release; saying otherwise
  // would promise the user something the uninstall does not do.
  if (image?.type === "tool") {
    return `“${instance.name}” is stopped and disabled in the project container. Installed packages and any data it wrote stay there.`;
  }
  if (instance.scope === "global") {
    return `“${instance.name}” runs in its own container, which is deleted along with its data. The host port ${instance.bindAddress}:${instance.externalPort} is released.`;
  }
  return `“${instance.name}” is stopped and disabled, and the host port ${instance.bindAddress}:${instance.externalPort} is released. Installed packages and data stay in the project container.`;
}

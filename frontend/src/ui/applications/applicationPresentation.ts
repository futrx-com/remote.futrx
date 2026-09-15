import type { AppApplication, AppInstance } from "../../models/application";

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
    target.add(instance.applicationId);
  }
  return { installedImageIds, failedImageIds };
}

// Whether installing this application put anything in a container.
//
// The answer comes from the server's package-layout detection. An application not in the catalog yet keeps
// the historical service presentation until the catalog finishes loading.
export function hasContainer(application: AppApplication | undefined): boolean {
  return application?.needsContainer ?? true;
}

// Whether this application binds a host port, which is what the port row and the
// credentials panel are about.
export function hasPortBinding(application: AppApplication | undefined): boolean {
  return application?.needsPort ?? true;
}

// The line shown in place of the port row for an application that has no port.
export function instanceSummary(
  application: AppApplication | undefined,
  running: boolean,
): string {
  if (application && hasContainer(application) && !hasPortBinding(application)) {
    return running
      ? "Infrastructure installed in its target container. Nothing is exposed."
      : "Infrastructure stopped. Start it to run it in its target container.";
  }
  if (application && !hasContainer(application)) {
    return running ? "Application enabled without infrastructure." : "Application stopped.";
  }
  return running
    ? "Running in a container."
    : "Stopped. Start it to run it again.";
}

/**
 * A stopped copy whose app has since moved to a new version. An upload
 * re-provisions the copies that are running and leaves stopped ones alone, so
 * this is the one state where the version an instance holds and the version
 * the catalog offers can drift — and starting it is what closes the gap.
 *
 * Only apps that reach a container can be stale.
 */
export function pendingUpgradeVersion(
  instance: AppInstance,
  application: AppApplication | undefined,
): string | null {
  if (!application || !hasContainer(application) || instance.status !== "stopped") return null;
  if (!application.version || application.version === instance.applicationVersion) return null;
  return application.version;
}

export function uninstallConsequence(
  instance: AppInstance,
  application: AppApplication | undefined,
): string {
  if (!hasContainer(application)) {
    return `“${instance.name}” is disabled. Nothing is removed from any container.`;
  }
  // Portless infrastructure holds no host port, so there is none to release; saying otherwise
  // would promise the user something the uninstall does not do.
  if (application && !hasPortBinding(application)) {
    return `“${instance.name}” is stopped and disabled in the project container. Installed packages and any data it wrote stay there.`;
  }
  if (instance.scope === "global") {
    return `“${instance.name}” runs in its own container, which is deleted along with its data. The host port ${instance.bindAddress}:${instance.externalPort} is released.`;
  }
  return `“${instance.name}” is stopped and disabled, and the host port ${instance.bindAddress}:${instance.externalPort} is released. Installed packages and data stay in the project container.`;
}

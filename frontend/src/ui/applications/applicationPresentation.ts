import type { AppApplication, AppInstance } from "../../models/application";

export interface CatalogInstallationState {
  installedApplicationIds: Set<string>;
  failedApplicationIds: Set<string>;
}

// Catalog cards distinguish completed installs from failed attempts: working
// instances disable installation, while failed ones offer the existing retry
// workflow. Keep both sets because persisted data may contain both states and
// the installed state must continue to win in the renderer.
export function catalogInstallationState(
  instances: AppInstance[],
): CatalogInstallationState {
  const installedApplicationIds = new Set<string>();
  const failedApplicationIds = new Set<string>();
  for (const instance of instances) {
    const target = instance.status === "error"
      ? failedApplicationIds
      : installedApplicationIds;
    target.add(instance.applicationId);
  }
  return { installedApplicationIds, failedApplicationIds };
}

// Whether installing this application put anything in a container.
//
// The answer comes from the server's layout detection. An application not in the catalog yet keeps
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
  if (application?.backend) {
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
    return `“${instance.name}” stops contributing to the interface, and any plugin it runs is stopped and its data deleted. Nothing is removed from any container.`;
  }
  // Infrastructure without a port has no host port to release; saying otherwise
  // would promise the user something the uninstall does not do.
  if (application && !hasPortBinding(application)) {
    return `“${instance.name}” is stopped and disabled in the project container. Installed packages and any data it wrote stay there.`;
  }
  if (instance.scope === "global") {
    return `“${instance.name}” runs in its own container, which is deleted along with its data. The host port ${instance.bindAddress}:${instance.externalPort} is released.`;
  }
  return `“${instance.name}” is stopped and disabled, and the host port ${instance.bindAddress}:${instance.externalPort} is released. Installed packages and data stay in the project container.`;
}

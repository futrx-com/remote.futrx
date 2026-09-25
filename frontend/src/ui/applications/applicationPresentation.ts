import type {
  AppApplication,
  AppInstance,
  AppPackage,
  AppPackageInstall,
  AppScope,
  AppUpgradeOutcome,
} from "../../models/application";

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

// Only completed installs have a lifecycle action. Error/installing records
// are attempts: retrying them goes through Install so partial state is cleaned
// up before a fresh instance is created.
export function instanceLifecycleAction(
  status: AppInstance["status"],
): "start" | "stop" | null {
  if (status === "running") return "stop";
  if (status === "stopped") return "start";
  return null;
}

// A failed install can be persisted before target/port resolution finishes.
// Catalog capability alone therefore cannot prove that the row has usable
// connection details.
export function hasAssignedConnection(
  instance: AppInstance,
  application: AppApplication | undefined,
): boolean {
  return hasPortBinding(application) &&
    (instance.containerName ?? "").trim() !== "" &&
    instance.internalPort > 0 &&
    instance.externalPort > 0 &&
    (instance.bindAddress ?? "").trim() !== "";
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
      ? "Backend extension — a Go backend runs on the server, not in a container."
      : "Backend extension — stopped. Start it to run its Go backend.";
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
    return `“${instance.name}” stops contributing to the interface, and any backend it runs is stopped and its data deleted. Nothing is removed from any container.`;
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

/**
 * Where this app can actually be installed. It is the first thing to say about
 * a package listed on a page about global applications: a project-only app is
 * in this catalog but cannot be installed here, and a row that omitted that
 * would be claiming otherwise by silence.
 */
export function packageScopes(pkg: AppPackage, viewing: AppScope): string {
  const scopes = pkg.scopes ?? [];
  const global = scopes.includes("global");
  const project = scopes.includes("project");
  if (!global && !project) return "Declares no scope, so it cannot be installed.";
  if (global && project) {
    // "Here, and in every other project" is a sentence only a project page can
    // say. Settings is not standing in a project, so from there the two scopes
    // are named rather than pointed at — the reader is told where the app goes,
    // not that it goes where they already are.
    return viewing === "project"
      ? "Installs here, and in every other project — it offers both scopes."
      : "Installs globally, or inside a project.";
  }
  if (global) {
    return viewing === "global"
      ? "Installs globally only — from this page."
      : "Installs globally only — not in a project. Use Settings → Applications.";
  }
  return viewing === "project"
    ? "Installs inside a project — including this one, from the list below."
    : "Installs inside a project only — from that project's Containers → Applications.";
}

/**
 * Where to go next after an upload that installed nothing yet. Saying "install
 * it below" for a project-only app would point at a grid that deliberately
 * does not list it.
 */
export function whereToInstall(pkg: AppPackage, viewing: AppScope): string {
  const scopes = pkg.scopes ?? [];
  // An app declaring nothing installs nowhere, so it is answered before either
  // "somewhere else" branch — otherwise it would be sent to a page that cannot
  // install it either.
  if (scopes.length === 0) return "It declares no scope, so nothing can install it.";
  if (scopes.includes(viewing)) return "Install it below.";
  return viewing === "project"
    ? "It installs globally only — open Settings → Applications."
    : "It installs inside a project — open that project's Containers → Applications.";
}

/** Where this package is installed right now, in the operator's words. */
export function describeInstalls(pkg: AppPackage): string {
  const installs = pkg.installs ?? [];
  const where = installs.map((install) =>
    install.scope === "project" ? `project ${install.projectId}` : "globally",
  );
  return `Installed ${where.join(", ")}.`;
}

/** One upgraded copy, named the way the operator would look for it. */
export function describeOutcome(outcome: AppUpgradeOutcome): string {
  return outcome.scope === "project"
    ? `${outcome.name} in project ${outcome.projectId}`
    : outcome.name;
}

/**
 * What removing a package costs, in one line under the dialog title. Removing
 * one that is still installed has to uninstall those copies first — otherwise
 * they would point at a catalog entry that no longer exists — and that is real
 * destruction, so it is said before the operator agrees rather than after.
 */
export function packageRemovalSummary(installs: AppPackageInstall[]): string {
  if (installs.length === 0) return "The uploaded package is deleted from this server.";
  return `${installs.length} installed ${
    installs.length === 1 ? "copy" : "copies"
  } will be uninstalled first.`;
}

/** The confirm button, named for everything it does and not just the last bit. */
export function packageRemovalConfirmLabel(installs: AppPackageInstall[]): string {
  return installs.length > 0 ? "Uninstall and remove" : "Remove";
}

/**
 * How far the removal reaches, said from the page the operator is standing on.
 * There is one catalog, so removing from a project removes for every project.
 */
export function packageRemovalReach(viewing: AppScope): string {
  return viewing === "project" ? "every project on this server" : "the whole server";
}

/** One copy the removal will take down, as the dialog lists it. */
export function describeRemovedInstall(install: AppPackageInstall): string {
  return install.scope === "project"
    ? `in project ${install.projectId}`
    : "installed globally";
}

/**
 * How many uploaded apps the catalog holds, beside the heading. A listing that
 * failed is counted as neither: "none yet" would state the one thing the
 * request never established, and it is the answer an operator is most likely
 * to act on by uploading a package they already uploaded.
 */
export function packageCountLabel(count: number, failed: boolean): string {
  if (failed) return "could not be listed";
  if (count === 0) return "none yet";
  return `${count} ${count === 1 ? "package" : "packages"}`;
}

/** Enough provenance to tell two uploads of the same app apart. */
export function packageSummary(pkg: AppPackage): string {
  const parts: string[] = [];
  if (pkg.uploadedAt) {
    parts.push(`uploaded ${new Date(pkg.uploadedAt * 1000).toLocaleString()}`);
  }
  if (pkg.uploadedBy) parts.push(`by ${pkg.uploadedBy}`);
  if (pkg.size) parts.push(formatBytes(pkg.size));
  return parts.join(" · ");
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

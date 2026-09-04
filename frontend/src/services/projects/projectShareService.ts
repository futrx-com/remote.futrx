import {
  PROJECT_PREVIEW_PORT_RANGE,
  PROJECT_RESERVED_PREVIEW_PORTS,
} from "../../config/project.ts";
import type {
  ContainerApp,
  ProjectShare,
  SharePortRow,
} from "../../models/project.ts";

class ProjectShareService {
  private readonly reservedPorts = new Set<number>(
    Object.values(PROJECT_RESERVED_PREVIEW_PORTS),
  );

  /**
   * Every discovered listener that can be shared, plus linked ports whose app
   * is no longer listening. Rows are unique and ordered by port.
   */
  portRows(apps: ContainerApp[], shares: ProjectShare[]): SharePortRow[] {
    const rows = new Map<number, SharePortRow>();
    for (const app of apps) {
      if (!this.isShareablePort(app.port) || rows.has(app.port)) continue;
      rows.set(app.port, { port: app.port, process: app.process, shareCount: 0 });
    }
    for (const share of shares) {
      if (!this.isShareablePort(share.port)) continue;
      const row = rows.get(share.port) ?? { port: share.port, shareCount: 0 };
      rows.set(share.port, { ...row, shareCount: row.shareCount + 1 });
    }
    return [...rows.values()].sort((left, right) => left.port - right.port);
  }

  /** Live links, newest first — the same order the backend returns. */
  live(shares: ProjectShare[], now: number): ProjectShare[] {
    return shares
      .filter((share) => !this.isExpired(share, now))
      .sort((left, right) => right.createdAt - left.createdAt);
  }

  add(shares: ProjectShare[], share: ProjectShare): ProjectShare[] {
    return [share, ...shares.filter((existing) => existing.id !== share.id)];
  }

  remove(shares: ProjectShare[], shareId: string): ProjectShare[] {
    return shares.filter((share) => share.id !== shareId);
  }

  /** Human phrasing for how long a link has left. */
  formatExpiry(expiresAt: number, now: number): string {
    const remaining = expiresAt - now;
    if (remaining <= 0) return "expired";
    const minutes = Math.floor(remaining / 60_000);
    if (minutes < 60) return `expires in ${Math.max(1, minutes)}m`;
    const hours = Math.floor(minutes / 60);
    if (hours < 48) return `expires in ${hours}h`;
    return `expires in ${Math.floor(hours / 24)}d`;
  }

  describeCount(count: number): string {
    if (count === 0) return "No active public links";
    return `${count} active public link${count === 1 ? "" : "s"}`;
  }

  private isShareablePort(port: number): boolean {
    return (
      Number.isInteger(port) &&
      port >= PROJECT_PREVIEW_PORT_RANGE.min &&
      port <= PROJECT_PREVIEW_PORT_RANGE.max &&
      !this.reservedPorts.has(port)
    );
  }

  private isExpired(share: ProjectShare, now: number): boolean {
    return share.expiresAt <= now;
  }
}

export const projectShareService = new ProjectShareService();

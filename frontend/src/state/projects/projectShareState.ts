import type { ContainerApp, ProjectShare } from "../../models/project";

/** Lifetimes offered in the share UI. The backend caps anything above 30 days. */
export const SHARE_TTL_OPTIONS: ReadonlyArray<{ hours: number; label: string }> = [
  { hours: 1, label: "1 hour" },
  { hours: 24, label: "24 hours" },
  { hours: 168, label: "7 days" },
];

export const DEFAULT_SHARE_TTL_HOURS = 24;

/** The agent browser's noVNC port is never shareable; the backend refuses it too. */
export const AGENT_BROWSER_PORT = 6080;
/** Platform plumbing ports (IDE proxy, code-server, CDP) are never shareable either. */
export const RESERVED_SHARE_PORTS: ReadonlySet<number> = new Set([AGENT_BROWSER_PORT, 8842, 8081, 9222]);

const MIN_SHARE_PORT = 1024;
const MAX_SHARE_PORT = 65535;

export interface SharePortRow {
  port: number;
  process?: string;
  /** How many live links already point at this port. */
  shareCount: number;
}

export function isShareablePort(port: number): boolean {
  return (
    Number.isInteger(port) &&
    port >= MIN_SHARE_PORT &&
    port <= MAX_SHARE_PORT &&
    !RESERVED_SHARE_PORTS.has(port)
  );
}

/**
 * The rows the Share panel offers: every discovered listener that can be
 * shared, plus any port that already has a link but is no longer listening, so
 * a link never becomes invisible just because its app stopped.
 */
export function shareablePortRows(
  apps: ContainerApp[],
  shares: ProjectShare[],
): SharePortRow[] {
  const rows = new Map<number, SharePortRow>();
  for (const app of apps) {
    if (!isShareablePort(app.port) || rows.has(app.port)) continue;
    rows.set(app.port, { port: app.port, process: app.process, shareCount: 0 });
  }
  for (const share of shares) {
    if (!isShareablePort(share.port)) continue;
    const row = rows.get(share.port) ?? { port: share.port, shareCount: 0 };
    rows.set(share.port, { ...row, shareCount: row.shareCount + 1 });
  }
  return [...rows.values()].sort((left, right) => left.port - right.port);
}

export function isShareExpired(share: ProjectShare, now: number): boolean {
  return share.expiresAt <= now;
}

/** Live links, newest first — the same order the backend returns. */
export function liveShares(shares: ProjectShare[], now: number): ProjectShare[] {
  return shares
    .filter((share) => !isShareExpired(share, now))
    .sort((left, right) => right.createdAt - left.createdAt);
}

export function addShare(shares: ProjectShare[], share: ProjectShare): ProjectShare[] {
  return [share, ...shares.filter((existing) => existing.id !== share.id)];
}

export function removeShare(shares: ProjectShare[], shareId: string): ProjectShare[] {
  return shares.filter((share) => share.id !== shareId);
}

/** Human phrasing for how long a link has left. */
export function formatShareExpiry(expiresAt: number, now: number): string {
  const remaining = expiresAt - now;
  if (remaining <= 0) return "expired";
  const minutes = Math.floor(remaining / 60_000);
  if (minutes < 60) return `expires in ${Math.max(1, minutes)}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `expires in ${hours}h`;
  return `expires in ${Math.floor(hours / 24)}d`;
}

export function describeShareCount(count: number): string {
  if (count === 0) return "No active public links";
  return `${count} active public link${count === 1 ? "" : "s"}`;
}

import { useMemo, useState } from "preact/hooks";
import type { ProjectShare } from "../../../models/project";
import type { SharesRecord } from "../../../state/projects/projectContainerRecords";
import {
  DEFAULT_SHARE_TTL_HOURS,
  SHARE_TTL_OPTIONS,
  formatShareExpiry,
  liveShares,
  shareablePortRows,
} from "../../../state/projects/projectShareState";
import { AlertCircle, Check, Clock, ExternalLink, X } from "../../primitives/icons";
import { Empty, Loading } from "./ProjectContainerPrimitives";

export function ProjectPreviewSharesSection({
  record,
  onCreate,
  onRevoke,
}: {
  record: SharesRecord;
  onCreate: (port: number, ttlHours: number, label?: string) => Promise<ProjectShare>;
  onRevoke: (shareId: string) => Promise<void>;
}) {
  // Held in the section, not the row, so switching ports does not lose the
  // link the operator has not copied yet.
  const [issued, setIssued] = useState<ProjectShare | null>(null);
  const shares = record.data ?? [];
  const rows = useMemo(
    () => shareablePortRows(record.apps ?? [], shares),
    [record.apps, shares]
  );

  const createFor = async (port: number, ttlHours: number) => {
    const created = await onCreate(port, ttlHours);
    setIssued(created);
  };

  const revoke = async (shareId: string) => {
    await onRevoke(shareId);
    setIssued((current) => (current?.id === shareId ? null : current));
  };

  return (
    <>
      {record.error && (
        <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
          <AlertCircle class="w-4 h-4 mt-0.5 flex-none text-accent-red" />
          <div class="text-accent-red break-words">{record.error}</div>
        </div>
      )}

      {issued?.url && <IssuedLink share={issued} onDismiss={() => setIssued(null)} />}

      {record.loading && !record.data ? (
        <Loading text="Loading preview ports…" />
      ) : rows.length === 0 ? (
        <Empty
          text="No shareable app is listening. Start the project's dev server, then refresh."
          compact
        />
      ) : (
        <div class="space-y-2">
          {rows.map((row) => (
            <SharePortRow
              key={row.port}
              port={row.port}
              process={row.process}
              shareCount={row.shareCount}
              onShare={(ttlHours) => createFor(row.port, ttlHours)}
            />
          ))}
        </div>
      )}

      <ActiveSharesList shares={shares} onRevoke={revoke} />

      <p class="text-[11.5px] text-ink-400 leading-relaxed">
        Anyone holding a link can open that one preview port until it expires or you
        revoke it — no sign-in, no invitation. Links never reach the IDE, the Agent
        Browser, another port, or this application. The link is shown once; the server
        keeps only a hash of it.
      </p>
    </>
  );
}

function IssuedLink({
  share,
  onDismiss,
}: {
  share: ProjectShare;
  onDismiss: () => void;
}) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(share.url ?? "");
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  };

  return (
    <div class="rounded-md border border-accent-blue/30 bg-accent-blue/[0.08] px-3 py-2.5 space-y-2">
      <div class="flex items-center gap-2">
        <ExternalLink class="w-4 h-4 flex-none text-accent-blue" />
        <div class="text-[12.5px] font-semibold text-ink-50">
          Public link for port {share.port} — copy it now
        </div>
        <button
          type="button"
          onClick={onDismiss}
          class="h-7 w-7 ml-auto rounded text-ink-300 hover:text-ink-50 hover:bg-white/[0.08] grid place-items-center"
          aria-label="Hide link"
          title="Hide link"
        >
          <X class="w-3.5 h-3.5" />
        </button>
      </div>
      <div class="flex items-start gap-2">
        <code class="flex-1 min-w-0 text-[12px] font-mono text-ink-100 break-all">
          {share.url}
        </code>
        <button
          type="button"
          onClick={copy}
          class="h-8 px-2.5 rounded bg-accent-blue/80 hover:bg-accent-blue text-white text-[12px] font-medium inline-flex items-center gap-1.5 flex-none"
        >
          {copied ? <Check class="w-3.5 h-3.5" /> : null}
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <div class="text-[11.5px] text-ink-300">
        This is the only time the link is shown. Closing this leaves the link active —
        revoke it below if it was not copied.
      </div>
    </div>
  );
}

function SharePortRow({
  port,
  process,
  shareCount,
  onShare,
}: {
  port: number;
  process?: string;
  shareCount: number;
  onShare: (ttlHours: number) => Promise<void>;
}) {
  const [ttlHours, setTtlHours] = useState(DEFAULT_SHARE_TTL_HOURS);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const share = async () => {
    setBusy(true);
    setErr(null);
    try {
      await onShare(ttlHours);
    } catch (error) {
      setErr((error as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div class="rounded-md border border-white/[0.08] bg-white/[0.03] px-3 py-2 space-y-1">
      <div class="flex items-center gap-2 flex-wrap">
        <span class="font-mono text-[12.5px] text-ink-50">:{port}</span>
        {process && <span class="text-[11.5px] text-ink-400 truncate">{process}</span>}
        {shareCount > 0 && (
          <span class="text-[11px] text-ink-300 rounded px-1.5 py-0.5 bg-white/[0.06] border border-white/10">
            {shareCount} link{shareCount === 1 ? "" : "s"}
          </span>
        )}
        <div class="ml-auto flex items-center gap-2">
          <select
            value={String(ttlHours)}
            onChange={(event) =>
              setTtlHours(Number((event.target as HTMLSelectElement).value))
            }
            aria-label={`Link lifetime for port ${port}`}
            class="h-8 px-2 rounded border border-white/10 bg-black/30 text-[12px] text-ink-100 focus:outline-none focus:border-accent-blue/50"
          >
            {SHARE_TTL_OPTIONS.map((option) => (
              <option key={option.hours} value={String(option.hours)}>
                {option.label}
              </option>
            ))}
          </select>
          <button
            type="button"
            onClick={share}
            disabled={busy}
            class="h-8 px-3 rounded bg-accent-blue/80 hover:bg-accent-blue text-white text-[12px] font-medium disabled:opacity-50"
          >
            {busy ? "Creating…" : "Share"}
          </button>
        </div>
      </div>
      {err && <div class="text-[11.5px] text-accent-red">{err}</div>}
    </div>
  );
}

function ActiveSharesList({
  shares,
  onRevoke,
}: {
  shares: ProjectShare[];
  onRevoke: (shareId: string) => Promise<void>;
}) {
  const now = Date.now();
  const live = liveShares(shares, now);
  if (live.length === 0) return <Empty text="No active public links." compact />;
  return (
    <div class="space-y-2">
      {live.map((share) => (
        <ActiveShareRow
          key={share.id}
          share={share}
          now={now}
          onRevoke={() => onRevoke(share.id)}
        />
      ))}
    </div>
  );
}

function ActiveShareRow({
  share,
  now,
  onRevoke,
}: {
  share: ProjectShare;
  now: number;
  onRevoke: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const revoke = async () => {
    if (!confirm(`Revoke the public link for port ${share.port}?`)) return;
    setBusy(true);
    setErr(null);
    try {
      await onRevoke();
    } catch (error) {
      setErr((error as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div class="rounded-md border border-white/[0.08] bg-white/[0.03] px-3 py-2 space-y-1">
      <div class="flex items-center gap-2 min-w-0">
        <span class="font-mono text-[12.5px] text-ink-50">:{share.port}</span>
        {share.label && (
          <span class="text-[12px] text-ink-200 truncate" title={share.label}>
            {share.label}
          </span>
        )}
        <span class="text-[11px] text-ink-400 inline-flex items-center gap-1 whitespace-nowrap ml-auto">
          <Clock class="w-3 h-3" />
          {formatShareExpiry(share.expiresAt, now)}
        </span>
        <button
          type="button"
          onClick={revoke}
          disabled={busy}
          class="h-7 w-7 rounded text-ink-300 hover:text-accent-red hover:bg-white/[0.08] grid place-items-center disabled:opacity-50"
          aria-label={`Revoke the public link for port ${share.port}`}
          title="Revoke link"
        >
          <X class="w-3.5 h-3.5" />
        </button>
      </div>
      {share.createdBy && (
        <div class="text-[11px] text-ink-400 truncate">created by {share.createdBy}</div>
      )}
      {err && <div class="text-[11.5px] text-accent-red">{err}</div>}
    </div>
  );
}

import { useState } from "preact/hooks";
import type { ComponentChildren } from "preact";
import type { AuditEntry, AuditFilters } from "../../models/audit";
import { AlertCircle, Check, Download, Loader, RotateCcw, X } from "../primitives/icons";

// AuditLogSettings is the admin read view over the append-only trail. It is
// presentational: filtering and paging happen on the server, so this component
// only edits a filter draft, applies it, and renders whatever page arrived.
export function AuditLogSettings({
  entries,
  filters,
  loading,
  loadingMore,
  hasMore,
  error,
  exportUrl,
  onFiltersChange,
  onRefresh,
  onLoadMore,
}: {
  entries: AuditEntry[];
  filters: AuditFilters;
  loading: boolean;
  loadingMore: boolean;
  hasMore: boolean;
  error: string | null;
  exportUrl: string;
  onFiltersChange: (filters: AuditFilters) => void;
  onRefresh: () => Promise<void>;
  onLoadMore: () => Promise<void>;
}) {
  return (
    <section class="rounded-lg border border-white/10 bg-[#101318] overflow-hidden">
      <header class="px-4 py-3 flex items-start gap-3 border-b border-white/[0.06]">
        <div class="flex-1 min-w-0">
          <div class="text-[14.5px] font-semibold text-ink-50">Audit log</div>
          <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">
            Who signed in, changed projects, read secrets, ran agents, and changed
            settings. Entries are append-only and kept for the configured retention
            window.
          </div>
        </div>
        <div class="flex items-center gap-1 flex-none">
          <button
            type="button"
            onClick={() => void onRefresh()}
            disabled={loading}
            class="h-9 px-2.5 rounded-md inline-flex items-center gap-2 text-[12px] text-ink-200
                   hover:text-ink-50 hover:bg-white/[0.08] disabled:opacity-60"
          >
            <RotateCcw class={`w-3.5 h-3.5 ${loading ? "animate-spin" : ""}`} />
            <span class="hidden sm:inline">Refresh</span>
          </button>
          <a
            href={exportUrl}
            download="audit-log.jsonl"
            class="h-9 px-2.5 rounded-md inline-flex items-center gap-2 text-[12px] text-ink-200
                   hover:text-ink-50 hover:bg-white/[0.08]"
            title="Download the matching date range as JSONL"
          >
            <Download class="w-3.5 h-3.5" />
            <span class="hidden sm:inline">Export</span>
          </a>
        </div>
      </header>

      <div class="p-3 space-y-3">
        <AuditFilterBar filters={filters} onApply={onFiltersChange} />

        {error && (
          <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
            <AlertCircle class="w-4 h-4 mt-0.5 flex-none text-accent-red" />
            <div class="text-accent-red break-words">{error}</div>
          </div>
        )}

        {loading && entries.length === 0 ? (
          <div class="rounded-md border border-white/10 bg-white/[0.03] px-3 py-8 flex items-center justify-center gap-2 text-[12.5px] text-ink-300">
            <Loader class="w-4 h-4 animate-spin" /> Loading audit entries…
          </div>
        ) : entries.length === 0 ? (
          <div class="rounded-md border border-white/10 bg-white/[0.03] px-3 py-2.5 text-[13px] text-ink-300">
            No audit entries match these filters.
          </div>
        ) : (
          <AuditTable entries={entries} />
        )}

        {hasMore && (
          <div class="flex justify-center">
            <button
              type="button"
              onClick={() => void onLoadMore()}
              disabled={loadingMore}
              class="h-8 px-3 rounded-md text-[12px] text-ink-300 hover:text-ink-100
                     hover:bg-white/[0.07] border border-white/10 disabled:opacity-50"
            >
              {loadingMore ? "Loading older entries…" : "Load older entries"}
            </button>
          </div>
        )}
      </div>
    </section>
  );
}

// AuditFilterBar keeps a local draft so typing a partial email or a
// half-entered date does not refetch on every keystroke.
function AuditFilterBar({
  filters,
  onApply,
}: {
  filters: AuditFilters;
  onApply: (filters: AuditFilters) => void;
}) {
  const [draft, setDraft] = useState<AuditFilters>(filters);
  const dirty =
    draft.actor !== filters.actor ||
    draft.action !== filters.action ||
    draft.from !== filters.from ||
    draft.to !== filters.to;
  const active = Boolean(filters.actor || filters.action || filters.from || filters.to);

  const update = (patch: Partial<AuditFilters>) =>
    setDraft((current) => ({ ...current, ...patch }));

  const submit = (event: Event) => {
    event.preventDefault();
    onApply(draft);
  };

  const clear = () => {
    const cleared: AuditFilters = { actor: "", action: "", from: "", to: "" };
    setDraft(cleared);
    onApply(cleared);
  };

  return (
    <form
      onSubmit={submit}
      class="rounded-md border border-white/10 bg-white/[0.03] p-2.5 space-y-2"
    >
      <div class="grid gap-2 sm:grid-cols-2">
        <FilterField label="Actor">
          <input
            type="text"
            value={draft.actor}
            onInput={(event) => update({ actor: (event.target as HTMLInputElement).value })}
            placeholder="someone@example.com"
            spellcheck={false}
            autoComplete="off"
            class="h-8 w-full rounded-md border border-white/10 bg-black/30 px-2 text-[12px] text-ink-100
                   placeholder-ink-400 focus:outline-none focus:border-accent-blue/60"
          />
        </FilterField>
        <FilterField label="Action">
          <input
            type="text"
            value={draft.action}
            onInput={(event) => update({ action: (event.target as HTMLInputElement).value })}
            placeholder="project.secret."
            spellcheck={false}
            autoComplete="off"
            class="h-8 w-full rounded-md border border-white/10 bg-black/30 px-2 text-[12px] text-ink-100
                   placeholder-ink-400 focus:outline-none focus:border-accent-blue/60"
          />
        </FilterField>
      </div>
      <div class="grid gap-2 sm:grid-cols-2">
        <FilterField label="From">
          <input
            type="datetime-local"
            value={draft.from}
            onInput={(event) => update({ from: (event.target as HTMLInputElement).value })}
            class="h-8 w-full rounded-md border border-white/10 bg-black/30 px-2 text-[12px] text-ink-100
                   focus:outline-none focus:border-accent-blue/60"
          />
        </FilterField>
        <FilterField label="To">
          <input
            type="datetime-local"
            value={draft.to}
            onInput={(event) => update({ to: (event.target as HTMLInputElement).value })}
            class="h-8 w-full rounded-md border border-white/10 bg-black/30 px-2 text-[12px] text-ink-100
                   focus:outline-none focus:border-accent-blue/60"
          />
        </FilterField>
      </div>
      <div class="flex items-center gap-2">
        <button
          type="submit"
          disabled={!dirty}
          class="h-8 px-3 rounded bg-accent-blue/80 hover:bg-accent-blue text-white text-[12px]
                 font-medium disabled:opacity-50"
        >
          Apply filters
        </button>
        {active && (
          <button
            type="button"
            onClick={clear}
            class="h-8 px-2.5 rounded-md inline-flex items-center gap-1.5 text-[12px] text-ink-300
                   hover:text-ink-100 hover:bg-white/[0.08]"
          >
            <X class="w-3.5 h-3.5" /> Clear
          </button>
        )}
        <span class="ml-auto text-[11px] text-ink-400">
          Action matches on prefix, so <code>project.</code> selects every project action.
        </span>
      </div>
    </form>
  );
}

function FilterField({ label, children }: { label: string; children: ComponentChildren }) {
  return (
    <label class="block">
      <span class="mb-1 block text-[10px] uppercase tracking-wide text-ink-500">{label}</span>
      {children}
    </label>
  );
}

function AuditTable({ entries }: { entries: AuditEntry[] }) {
  return (
    <div class="overflow-x-auto touch-scroll border border-white/10 rounded-lg">
      <table class="w-full text-[12px] border-collapse">
        <thead class="bg-white/[0.04]">
          <tr>
            <AuditHeaderCell>Time</AuditHeaderCell>
            <AuditHeaderCell>Actor</AuditHeaderCell>
            <AuditHeaderCell>Action</AuditHeaderCell>
            <AuditHeaderCell>Target</AuditHeaderCell>
            <AuditHeaderCell>IP</AuditHeaderCell>
            <AuditHeaderCell>Status</AuditHeaderCell>
          </tr>
        </thead>
        <tbody>
          {entries.map((entry) => (
            <tr key={auditRowKey(entry)}>
              <AuditCell>
                <span class="whitespace-nowrap text-ink-200" title={entry.at}>
                  {formatAuditTime(entry.at)}
                </span>
              </AuditCell>
              <AuditCell>
                <span class="text-ink-100 break-all" title={entry.actor.sub ?? ""}>
                  {entry.actor.email || "system"}
                </span>
                {entry.actor.isAdmin && (
                  <span class="ml-1.5 inline-flex items-center h-4 px-1 rounded text-[10px] text-accent-blue bg-accent-blue/[0.14]">
                    admin
                  </span>
                )}
              </AuditCell>
              <AuditCell>
                <span class="text-ink-100 whitespace-nowrap">{entry.action}</span>
              </AuditCell>
              <AuditCell>
                <AuditTargetCell entry={entry} />
              </AuditCell>
              <AuditCell>
                <span class="whitespace-nowrap text-ink-300">{entry.ip || "—"}</span>
              </AuditCell>
              <AuditCell>
                <AuditStatus entry={entry} />
              </AuditCell>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function AuditHeaderCell({ children }: { children: ComponentChildren }) {
  return (
    <th class="text-left px-3 py-1.5 font-semibold border-b border-white/10 text-ink-100 whitespace-nowrap">
      {children}
    </th>
  );
}

function AuditCell({ children }: { children: ComponentChildren }) {
  return <td class="px-3 py-1.5 border-b border-white/[0.06] align-top">{children}</td>;
}

function AuditTargetCell({ entry }: { entry: AuditEntry }) {
  const label = entry.target?.name || entry.target?.id;
  if (!label) return <span class="text-ink-400">—</span>;
  return (
    <span class="text-ink-200 break-all" title={entry.target?.id ?? ""}>
      {entry.target?.type ? `${entry.target.type}: ` : ""}
      {label}
    </span>
  );
}

function AuditStatus({ entry }: { entry: AuditEntry }) {
  if (entry.ok) {
    return (
      <span class="inline-flex items-center gap-1 text-accent-green whitespace-nowrap">
        <Check class="w-3 h-3" /> ok
      </span>
    );
  }
  return (
    <span class="inline-flex items-center gap-1 text-accent-red" title={entry.error ?? ""}>
      <AlertCircle class="w-3 h-3 flex-none" />
      <span class="break-all">{entry.error || "failed"}</span>
    </span>
  );
}

function auditRowKey(entry: AuditEntry): string {
  return `${entry.at}|${entry.action}|${entry.actor.email ?? ""}|${entry.target?.id ?? ""}`;
}

// Timestamps arrive as RFC3339 in UTC and are shown in the operator's local
// zone, which is what makes them comparable with the rest of the UI.
function formatAuditTime(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleString();
}

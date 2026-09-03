import { useState } from "preact/hooks";
import type { ProjectSecret } from "../../../models/project";
import type { SecretsRecord } from "../../../models/project";
import { useConfirm } from "../../../state/context/ConfirmContext";
import { AlertCircle, X } from "../../primitives/icons";
import { Empty, Loading } from "./ProjectContainerPrimitives";
import {
  formatUnixTime,
  hasNewlines,
  lineSummary,
} from "./projectContainerFormat";

export function ProjectSecretsSection({
  record,
  onSave,
  onDelete,
}: {
  record: SecretsRecord;
  onSave: (key: string, value: string) => Promise<void>;
  onDelete: (key: string) => Promise<void>;
}) {
  return (
    <>
      {record.error && (
        <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
          <AlertCircle class="w-4 h-4 mt-0.5 flex-none text-accent-red" />
          <div class="text-accent-red break-words">{record.error}</div>
        </div>
      )}
      <SecretEditor onSave={onSave} />
      <SecretsList list={record.data ?? []} loading={record.loading && !record.data} onSave={onSave} onDelete={onDelete} />
      <p class="text-[11.5px] text-ink-400 leading-relaxed">
        Secrets are passed to the selected agent CLI as <span class="font-mono">--env KEY=VALUE</span> on every prompt run. They never land in the container's filesystem and are not synced back from it.
      </p>
    </>
  );
}

function SecretEditor({
  onSave,
}: {
  onSave: (key: string, value: string) => Promise<void>;
}) {
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async (event: Event) => {
    event.preventDefault();
    const normalizedKey = key.trim();
    if (!normalizedKey) {
      setErr("Key is required.");
      return;
    }
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(normalizedKey)) {
      setErr("Key must match [A-Za-z_][A-Za-z0-9_]*");
      return;
    }
    setErr(null);
    setSubmitting(true);
    try {
      await onSave(normalizedKey, value);
      setKey("");
      setValue("");
    } catch (error) {
      setErr((error as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={submit} class="rounded-md border border-line bg-tint p-2.5 space-y-2">
      <div class="grid gap-2 sm:grid-cols-[1fr_2fr_auto] items-start">
        <input
          value={key}
          onInput={(event) => setKey((event.target as HTMLInputElement).value)}
          placeholder="KEY"
          class="h-9 px-2.5 rounded border border-line bg-inset text-[13px] font-mono text-ink-50 placeholder-ink-400 focus:outline-none focus:border-accent-blue/50"
        />
        <textarea
          value={value}
          onInput={(event) => setValue((event.target as HTMLTextAreaElement).value)}
          placeholder="value (multi-line OK — paste PEM keys, JSON, etc.)"
          rows={1}
          spellcheck={false}
          autoComplete="off"
          class="min-h-9 max-h-48 px-2.5 py-1.5 rounded border border-line bg-inset text-[13px] font-mono text-ink-50 placeholder-ink-400 focus:outline-none focus:border-accent-blue/50 resize-y leading-[1.45] overflow-y-auto"
          style={{ fieldSizing: "content" } as any}
        />
        <button
          type="submit"
          disabled={submitting}
          class="btn btn-primary btn-sm text-[13px] font-medium disabled:opacity-50"
        >
          {submitting ? "Saving…" : "Add"}
        </button>
      </div>
      {err && <div class="text-[11.5px] text-accent-red">{err}</div>}
    </form>
  );
}

function SecretsList({
  list,
  loading,
  onSave,
  onDelete,
}: {
  list: ProjectSecret[];
  loading: boolean;
  onSave: (key: string, value: string) => Promise<void>;
  onDelete: (key: string) => Promise<void>;
}) {
  if (loading) return <Loading text="Loading secrets…" />;
  if (list.length === 0) return <Empty text="No secrets yet." compact />;
  return (
    <div class="space-y-2">
      {list.map((secret) => (
        <SecretRow key={secret.key} secret={secret} onSave={onSave} onDelete={onDelete} />
      ))}
    </div>
  );
}

function SecretRow({
  secret,
  onSave,
  onDelete,
}: {
  secret: ProjectSecret;
  onSave: (key: string, value: string) => Promise<void>;
  onDelete: (key: string) => Promise<void>;
}) {
  const confirm = useConfirm();
  const [editing, setEditing] = useState(false);
  const [revealed, setRevealed] = useState(false);
  const [draft, setDraft] = useState(secret.value);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const save = async () => {
    setBusy(true);
    setErr(null);
    try {
      await onSave(secret.key, draft);
      setEditing(false);
    } catch (error) {
      setErr((error as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    const confirmed = await confirm({
      title: "Delete secret",
      description: "This action cannot be undone.",
      message: `${secret.key} will be removed from this project's environment.`,
      confirmLabel: "Delete secret",
    });
    if (!confirmed) return;
    setBusy(true);
    try {
      await onDelete(secret.key);
    } catch (error) {
      setErr((error as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div class="rounded-md border border-line bg-tint px-3 py-2 space-y-1">
      <div class="flex items-center gap-2 min-w-0">
        <span class="font-mono text-[12.5px] text-ink-50 truncate">{secret.key}</span>
        <span class="text-[11px] text-ink-400 ml-auto whitespace-nowrap">
          updated {formatUnixTime(secret.updatedAt)}
        </span>
      </div>
      {editing ? (
        <div class="flex items-start gap-2 flex-wrap">
          <textarea
            value={draft}
            onInput={(event) => setDraft((event.target as HTMLTextAreaElement).value)}
            rows={1}
            spellcheck={false}
            class="flex-1 min-h-8 max-h-48 px-2 py-1 rounded border border-line bg-inset text-[12.5px] font-mono text-ink-50 focus:outline-none focus:border-accent-blue/50 resize-y leading-[1.45] overflow-y-auto"
            style={{ fieldSizing: "content" } as any}
          />
          <button
            type="button"
            onClick={() => setRevealed(!revealed)}
            class="h-8 px-2 rounded text-[11px] text-ink-300 hover:text-ink-100 hover:bg-tint-strong"
          >
            {revealed ? "hide" : "show"}
          </button>
          <button
            type="button"
            onClick={save}
            disabled={busy}
            class="btn btn-primary btn-sm text-[12px] font-medium disabled:opacity-50"
          >
            Save
          </button>
          <button
            type="button"
            onClick={() => {
              setEditing(false);
              setDraft(secret.value);
              setErr(null);
            }}
            class="h-8 px-2 rounded text-[12px] text-ink-300 hover:text-ink-100 hover:bg-tint-strong"
          >
            Cancel
          </button>
        </div>
      ) : (
        <div class="flex items-center gap-2">
          <code class="flex-1 text-[12.5px] font-mono text-ink-100 break-all min-w-0 whitespace-pre-wrap max-h-48 overflow-y-auto">
            {revealed
              ? secret.value
              : hasNewlines(secret.value)
                ? lineSummary(secret.value)
                : "•".repeat(Math.min(20, secret.value.length || 6))}
          </code>
          <button
            type="button"
            onClick={() => setRevealed(!revealed)}
            class="h-7 px-2 rounded text-[11px] text-ink-300 hover:text-ink-100 hover:bg-tint-strong"
          >
            {revealed ? "hide" : "show"}
          </button>
          <button
            type="button"
            onClick={() => setEditing(true)}
            class="h-7 px-2 rounded text-[11px] text-ink-300 hover:text-ink-100 hover:bg-tint-strong"
          >
            edit
          </button>
          <button
            type="button"
            onClick={remove}
            disabled={busy}
            class="h-7 w-7 rounded text-ink-300 hover:text-accent-red hover:bg-tint-strong grid place-items-center disabled:opacity-50"
            aria-label="Delete"
            title="Delete"
          >
            <X class="w-3.5 h-3.5" />
          </button>
        </div>
      )}
      {err && <div class="text-[11.5px] text-accent-red">{err}</div>}
    </div>
  );
}

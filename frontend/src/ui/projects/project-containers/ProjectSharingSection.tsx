import { useState } from "preact/hooks";
import type { AccessRecord } from "../../../models/project";
import { useConfirm } from "../../../state/context/ConfirmContext";
import { AlertCircle, X } from "../../primitives/icons";
import { Empty, Loading } from "./ProjectContainerPrimitives";

export function ProjectSharingSection({
  record,
  onAdd,
  onRemove,
}: {
  record: AccessRecord;
  onAdd: (email: string) => Promise<void>;
  onRemove: (email: string) => Promise<void>;
}) {
  return (
    <>
      {record.error && (
        <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
          <AlertCircle class="w-4 h-4 mt-0.5 flex-none text-accent-red" />
          <div class="text-accent-red break-words">{record.error}</div>
        </div>
      )}
      <AddMemberForm onAdd={onAdd} />
      <MembersList
        members={record.data ?? []}
        loading={record.loading && !record.data}
        onRemove={onRemove}
      />
      <p class="text-[11.5px] text-ink-400 leading-relaxed">
        Members can use this project — terminal, chats, secrets, uploads, browser. To add someone here they must first appear in the global Users panel (Account &rarr; Users).
      </p>
    </>
  );
}

function AddMemberForm({
  onAdd,
}: {
  onAdd: (email: string) => Promise<void>;
}) {
  const [email, setEmail] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async (event: Event) => {
    event.preventDefault();
    const normalizedEmail = email.trim().toLowerCase();
    if (!normalizedEmail) {
      setErr("Email is required.");
      return;
    }
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(normalizedEmail)) {
      setErr("That doesn't look like an email.");
      return;
    }
    setErr(null);
    setSubmitting(true);
    try {
      await onAdd(normalizedEmail);
      setEmail("");
    } catch (error) {
      setErr((error as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={submit} class="rounded-md border border-line bg-tint p-2.5 space-y-2">
      <div class="grid gap-2 sm:grid-cols-[1fr_auto] items-center">
        <input
          type="email"
          value={email}
          onInput={(event) => setEmail((event.target as HTMLInputElement).value)}
          placeholder="someone@example.com"
          spellcheck={false}
          autoComplete="off"
          class="h-9 px-2.5 rounded border border-line bg-inset text-[13px] text-ink-50 placeholder-ink-400 focus:outline-none focus:border-accent-blue/50"
        />
        <button
          type="submit"
          disabled={submitting}
          class="btn btn-primary btn-sm text-[13px] font-medium disabled:opacity-50"
        >
          {submitting ? "Adding…" : "Add"}
        </button>
      </div>
      {err && <div class="text-[11.5px] text-accent-red">{err}</div>}
    </form>
  );
}

function MembersList({
  members,
  loading,
  onRemove,
}: {
  members: string[];
  loading: boolean;
  onRemove: (email: string) => Promise<void>;
}) {
  if (loading) return <Loading text="Loading members…" />;
  if (members.length === 0) return <Empty text="No members yet." compact />;
  return (
    <div class="space-y-2">
      {members.map((member) => (
        <MemberRow key={member} email={member} onRemove={() => onRemove(member)} />
      ))}
    </div>
  );
}

function MemberRow({
  email,
  onRemove,
}: {
  email: string;
  onRemove: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const confirm = useConfirm();

  const remove = async () => {
    const confirmed = await confirm({
      title: "Remove member",
      description: "They lose access to this project immediately.",
      message: `${email} will no longer see this project or its chats.`,
      confirmLabel: "Remove member",
    });
    if (!confirmed) return;
    setBusy(true);
    setErr(null);
    try {
      await onRemove();
    } catch (error) {
      setErr((error as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div class="rounded-md border border-line bg-tint px-3 py-2 space-y-1">
      <div class="flex items-center gap-2 min-w-0">
        <span class="text-[12.5px] text-ink-50 truncate" title={email}>
          {email}
        </span>
        <button
          type="button"
          onClick={remove}
          disabled={busy}
          class="h-7 w-7 ml-auto rounded text-ink-300 hover:text-accent-red hover:bg-tint-strong grid place-items-center disabled:opacity-50"
          aria-label={`Remove ${email}`}
          title="Remove member"
        >
          <X class="w-3.5 h-3.5" />
        </button>
      </div>
      {err && <div class="text-[11.5px] text-accent-red">{err}</div>}
    </div>
  );
}

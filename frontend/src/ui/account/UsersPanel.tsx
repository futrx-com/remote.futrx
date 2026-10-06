import { useState } from "preact/hooks";
import type { RbacBinding, RbacRole } from "../../models/rbac";
import type { User, UserRole } from "../../models/user";
import { DEFAULT_USER_ROLE } from "../../config/constants/user-roles";
import { useConfirm } from "../../state/context/ConfirmContext";
import { AlertCircle, Check, Loader, X } from "../primitives/icons";
import { RoleSelect } from "./RoleSelect";
import { boundCustomRoles, type RoleChoice } from "./roleChoice";

interface UsersPanelProps {
  currentEmail: string;
  isAdmin: boolean;
  users: User[] | null;
  loading: boolean;
  error: string | null;
  canAddUsers: boolean;
  customRoles: RbacRole[];
  bindings: RbacBinding[];
  onAdd: (email: string, role: UserRole) => Promise<void>;
  onRemove: (email: string) => Promise<void>;
  onSetRole: (email: string, role: UserRole) => Promise<void>;
  onBindRole: (email: string, roleId: string) => Promise<void>;
  onUnbindRole: (email: string, roleId: string) => Promise<void>;
}

// UsersPanel is the admin-only directory for sign-in eligibility. Mirrors
// the SecretsBody pattern: collapsible card, add-form on top, row actions
// on the right. Renders a friendly notice (instead of hard-failing) when
// the caller isn't an admin so non-admins who reach the account page still
// see something sensible.
export function UsersPanel({
  currentEmail,
  isAdmin,
  users,
  loading,
  error,
  canAddUsers,
  customRoles,
  bindings,
  onAdd,
  onRemove,
  onSetRole,
  onBindRole,
  onUnbindRole,
}: UsersPanelProps) {
  const confirm = useConfirm();

  const removeUser = async (email: string) => {
    await confirm({
      title: "Remove user",
      description: "They lose access immediately.",
      message: `${email} will no longer be able to sign in or reach any shared project.`,
      confirmLabel: "Remove user",
      pendingLabel: "Removing…",
      action: () => onRemove(email),
    });
  };

  if (!isAdmin) {
    return (
      <section class="rounded-card border border-line bg-surface p-4 text-[13px] text-ink-300">
        Users are admin-only. Ask your admin to add or remove people.
      </section>
    );
  }

  return (
    <section class="rounded-card border border-line bg-surface overflow-hidden">
      <header class="px-4 py-3 flex items-start gap-3 border-b border-line">
        <div class="flex-1 min-w-0">
          <div class="text-[14.5px] font-semibold text-ink-50">Users</div>
          <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">
            Anyone listed here can sign in. Admins manage users and delete
            projects; members can be added to specific projects.
          </div>
        </div>
        {loading && <Loader class="w-4 h-4 mt-2 text-ink-300 animate-spin" />}
      </header>

      <div class="p-3 space-y-3">
        {error && (
          <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
            <AlertCircle class="w-4 h-4 mt-0.5 flex-none text-accent-red" />
            <div class="text-accent-red break-words">{error}</div>
          </div>
        )}
        {canAddUsers ? (
          <AddUserForm
            customRoles={customRoles}
            onAdd={onAdd}
            onBindRole={onBindRole}
          />
        ) : (
          <div class="rounded-md border border-accent-yellow/25 bg-accent-yellow/[0.08] px-3 py-2.5 text-[12.5px] text-accent-yellow">
            Configure Google sign-in above before adding users.
          </div>
        )}
        <UserList
          users={users ?? []}
          loading={loading && users == null}
          currentEmail={currentEmail}
          onRemove={removeUser}
          customRoles={customRoles}
          bindings={bindings}
          onSetRole={onSetRole}
          onBindRole={onBindRole}
          onUnbindRole={onUnbindRole}
        />
      </div>
    </section>
  );
}

function AddUserForm({
  customRoles,
  onAdd,
  onBindRole,
}: {
  customRoles: RbacRole[];
  onAdd: (email: string, role: UserRole) => Promise<void>;
  onBindRole: (email: string, roleId: string) => Promise<void>;
}) {
  const [email, setEmail] = useState("");
  const [choice, setChoice] = useState<RoleChoice>({
    kind: "base",
    role: DEFAULT_USER_ROLE,
  });
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async (e: Event) => {
    e.preventDefault();
    const em = email.trim().toLowerCase();
    if (!em) {
      setErr("Email is required.");
      return;
    }
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(em)) {
      setErr("That doesn't look like an email.");
      return;
    }
    setErr(null);
    setSubmitting(true);
    try {
      await onAdd(em, choice.kind === "base" ? choice.role : DEFAULT_USER_ROLE);
      setEmail("");
      setChoice({ kind: "base", role: DEFAULT_USER_ROLE });
      if (choice.kind === "custom") await onBindRole(em, choice.roleId);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form
      onSubmit={submit}
      class="rounded-md border border-line bg-tint p-2.5 space-y-2"
    >
      <div class="grid gap-2 sm:grid-cols-[2fr_auto_auto] items-center">
        <input
          type="email"
          value={email}
          onInput={(e) => setEmail((e.target as HTMLInputElement).value)}
          placeholder="someone@example.com"
          spellcheck={false}
          autoComplete="off"
          class="h-9 px-2.5 rounded border border-line bg-inset text-[13px] text-ink-50 placeholder-ink-400 focus:outline-none focus:border-accent-blue/50"
        />
        <RoleSelect
          value={choice}
          customRoles={customRoles}
          onSelect={setChoice}
          ariaLabel="Role for new user"
          class="h-9 px-2 rounded border border-line bg-inset text-[13px] text-ink-50 focus:outline-none focus:border-accent-blue/50"
        />
        <button
          type="submit"
          disabled={submitting}
          class="btn btn-primary btn-sm disabled:opacity-50"
        >
          {submitting ? "Adding…" : "Add"}
        </button>
      </div>
      {err && <div class="text-[11.5px] text-accent-red">{err}</div>}
    </form>
  );
}

function UserList({
  users,
  loading,
  currentEmail,
  customRoles,
  bindings,
  onRemove,
  onSetRole,
  onBindRole,
  onUnbindRole,
}: {
  users: User[];
  loading: boolean;
  currentEmail: string;
  customRoles: RbacRole[];
  bindings: RbacBinding[];
  onRemove: (email: string) => Promise<void>;
  onSetRole: (email: string, role: UserRole) => Promise<void>;
  onBindRole: (email: string, roleId: string) => Promise<void>;
  onUnbindRole: (email: string, roleId: string) => Promise<void>;
}) {
  if (loading) {
    return (
      <div class="rounded-md border border-line bg-tint px-3 py-4 text-center text-[12.5px] text-ink-300">
        Loading users…
      </div>
    );
  }
  if (users.length === 0) {
    return (
      <div class="rounded-md border border-line bg-tint px-3 py-2.5 text-[13px] text-ink-300">
        No users yet.
      </div>
    );
  }
  return (
    <div class="space-y-2">
      {users.map((u) => (
        <UserRow
          key={u.email}
          user={u}
          isSelf={u.email === currentEmail.toLowerCase()}
          customRoles={customRoles}
          boundRoles={boundCustomRoles(bindings, customRoles, u.email)}
          onRemove={() => onRemove(u.email)}
          onSetRole={(r) => onSetRole(u.email, r)}
          onBindRole={(roleId) => onBindRole(u.email, roleId)}
          onUnbindRole={(roleId) => onUnbindRole(u.email, roleId)}
        />
      ))}
    </div>
  );
}

function UserRow({
  user,
  isSelf,
  customRoles,
  boundRoles,
  onRemove,
  onSetRole,
  onBindRole,
  onUnbindRole,
}: {
  user: User;
  isSelf: boolean;
  customRoles: RbacRole[];
  boundRoles: RbacRole[];
  onRemove: () => Promise<void>;
  onSetRole: (role: UserRole) => Promise<void>;
  onBindRole: (roleId: string) => Promise<void>;
  onUnbindRole: (roleId: string) => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const run = async (action: () => Promise<void>) => {
    setBusy(true);
    setErr(null);
    try {
      await action();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const changeRole = (next: RoleChoice) => {
    if (next.kind === "custom") return run(() => onBindRole(next.roleId));
    if (next.role === user.role) return;
    return run(() => onSetRole(next.role));
  };

  return (
    <div class="rounded-md border border-line bg-tint px-3 py-2 space-y-1">
      <div class="flex items-center gap-2 min-w-0">
        <span class="text-[12.5px] text-ink-50 truncate" title={user.email}>
          {user.email}
        </span>
        <span
          class={`inline-flex items-center h-5 px-1.5 rounded text-[11px] font-medium ${
            user.role === "admin"
              ? "text-accent-blue bg-accent-blue/[0.14]"
              : "text-ink-300 bg-tint"
          }`}
        >
          {user.role}
        </span>
        {boundRoles.map((role) => (
          <span
            key={role.id}
            class="inline-flex items-center h-5 pl-1.5 pr-0.5 rounded text-[11px] font-medium text-accent-blue bg-accent-blue/[0.14]"
            title={role.description || role.name}
          >
            {role.name}
            <button
              type="button"
              onClick={() => run(() => onUnbindRole(role.id))}
              disabled={busy}
              class="ml-0.5 h-4 w-4 grid place-items-center rounded hover:bg-tint-strong disabled:opacity-50"
              aria-label={`Unbind ${role.name} from ${user.email}`}
              title="Unbind role"
            >
              <X class="w-3 h-3" />
            </button>
          </span>
        ))}
        {isSelf && (
          <span class="inline-flex items-center h-5 px-1.5 rounded text-[11px] text-accent-green bg-accent-green/[0.10]">
            <Check class="w-3 h-3 mr-1" /> you
          </span>
        )}
        <div class="ml-auto flex items-center gap-1">
          <RoleSelect
            value={{ kind: "base", role: user.role }}
            customRoles={customRoles}
            boundRoleIds={new Set(boundRoles.map((role) => role.id))}
            disabled={busy}
            onSelect={changeRole}
            ariaLabel={`Role for ${user.email}`}
            title="Change role"
            class="h-7 px-1.5 rounded border border-line bg-inset text-[11.5px] text-ink-50 focus:outline-none focus:border-accent-blue/50 disabled:opacity-50"
          />
          <button
            type="button"
            onClick={() => run(onRemove)}
            disabled={busy}
            class="h-7 w-7 rounded text-ink-300 hover:text-accent-red hover:bg-tint-strong grid place-items-center disabled:opacity-50"
            aria-label={`Remove ${user.email}`}
            title="Remove user"
          >
            <X class="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
      {err && <div class="text-[11.5px] text-accent-red">{err}</div>}
    </div>
  );
}

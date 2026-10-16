import type {
  DirectoryUser,
  InviteInput,
  InviteResult,
  PasswordInviteResult,
  UserRole,
} from "../../models/user";
import { useIssuedCredentials } from "../../state/hooks/users/useIssuedCredentials";
import { useConfirm } from "../../state/context/ConfirmContext";
import { AlertCircle, Loader } from "../primitives/icons";
import { AddUserForm } from "./AddUserForm";
import { IssuedCredentialsCard } from "./IssuedCredentialsCard";
import { UserList } from "./UserList";

interface UsersPanelProps {
  currentEmail: string;
  isAdmin: boolean;
  users: DirectoryUser[] | null;
  loading: boolean;
  error: string | null;
  googleConfigured: boolean;
  smtpConfigured: boolean;
  /** The local administrator's email: the one account whose password cannot be reset here. */
  localAdminEmail: string;
  onAdd: (input: InviteInput) => Promise<InviteResult>;
  onResetPassword: (email: string, sendEmail: boolean) => Promise<PasswordInviteResult>;
  onRemove: (email: string) => Promise<void>;
  onSetRole: (email: string, role: UserRole) => Promise<void>;
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
  googleConfigured,
  smtpConfigured,
  localAdminEmail,
  onAdd,
  onResetPassword,
  onRemove,
  onSetRole,
}: UsersPanelProps) {
  const confirm = useConfirm();
  const credentials = useIssuedCredentials({ add: onAdd, resetTemporaryPassword: onResetPassword });

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
        <AddUserForm
          googleConfigured={googleConfigured}
          smtpConfigured={smtpConfigured}
          onAdd={credentials.invite}
        />
        {credentials.issued && (
          <IssuedCredentialsCard
            key={credentials.issued.serial}
            result={credentials.issued.result}
            onDone={credentials.dismiss}
          />
        )}
        <UserList
          users={users ?? []}
          loading={loading && users == null}
          currentEmail={currentEmail}
          localAdminEmail={localAdminEmail}
          smtpConfigured={smtpConfigured}
          onRemove={removeUser}
          onSetRole={onSetRole}
          onResetPassword={credentials.reset}
        />
      </div>
    </section>
  );
}

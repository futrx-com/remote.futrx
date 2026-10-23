import type { DirectoryUser, InviteResult, PasswordInviteResult, UserRole } from "../../../models/user.ts";

// The directory list as pure transitions, so the hook only chooses when to
// apply them.
export const userDirectoryState = {
  /** A just-invited user joins the list, kept in email order. */
  withInvited(current: DirectoryUser[] | null, result: InviteResult): DirectoryUser[] {
    const invited = result.method === "password";
    const added: DirectoryUser = {
      ...result.user,
      passwordSignIn: invited,
      passwordChangePending: invited,
    };
    return [...(current ?? []).filter((user) => user.email !== added.email), added].sort((left, right) =>
      left.email.localeCompare(right.email),
    );
  },

  /** The credentials a result carries, if it carries any. */
  issuedFrom(result: InviteResult): PasswordInviteResult | null {
    return result.method === "password" ? result : null;
  },

  /** A reset leaves the account on a temporary password, whatever it had before. */
  withTemporaryPassword(current: DirectoryUser[] | null, email: string): DirectoryUser[] {
    return (current ?? []).map((user) =>
      user.email === email ? { ...user, passwordSignIn: true, passwordChangePending: true } : user,
    );
  },

  withRole(current: DirectoryUser[] | null, email: string, role: UserRole): DirectoryUser[] {
    return (current ?? []).map((user) => (user.email === email ? { ...user, role } : user));
  },

  without(current: DirectoryUser[] | null, email: string): DirectoryUser[] {
    return (current ?? []).filter((user) => user.email !== email);
  },
};

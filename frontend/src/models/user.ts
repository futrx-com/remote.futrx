export type UserRole = "admin" | "member";

export interface User {
  email: string;
  role: UserRole;
  addedAt: number;
  addedBy?: string;
}

/** A user as the admin directory lists them, with how they can sign in. */
export interface DirectoryUser extends User {
  /** The account has a password credential (an invited or reset one). */
  passwordSignIn: boolean;
  /** The password is still the temporary one: the owner must replace it. */
  passwordChangePending: boolean;
}

/** How an invited account first signs in. */
export type InviteMethod = "google" | "password";

export interface InviteInput {
  email: string;
  role: UserRole;
  method: InviteMethod;
  /** Only meaningful for the password method. */
  sendEmail: boolean;
}

/** What happened to the credentials email of a password invite or reset. */
export type InvitationDelivery = "sent" | "failed" | "not_configured" | "not_requested";

interface IssuedPasswordBase {
  user: User;
  method: "password";
  /** The one place the plaintext exists on the client: keep it in component state only. */
  temporaryPassword: string;
}

export type PasswordInviteResult =
  | (IssuedPasswordBase & { delivery: "sent" | "not_configured" | "not_requested" })
  | (IssuedPasswordBase & { delivery: "failed"; deliveryError?: string });

/** A Google invite carries no credentials; a password invite or reset does. */
export type InviteResult = { method: "google"; user: User } | PasswordInviteResult;

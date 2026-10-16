import { requestJson } from "./apiRequest";
import type {
  DirectoryUser,
  InviteInput,
  InviteResult,
  PasswordInviteResult,
  User,
  UserRole,
} from "../models/user";
import { API_ROUTES } from "../config/routes";

export const userApi = {
  list: () => requestJson<DirectoryUser[]>("GET", API_ROUTES.users.collection),
  // A Google invite answers with the bare user; a password invite with the
  // issued credentials. Callers get one tagged shape for both.
  add: async (input: InviteInput): Promise<InviteResult> => {
    if (input.method === "google") {
      const user = await requestJson<User>("POST", API_ROUTES.users.collection, {
        email: input.email,
        role: input.role,
        method: "google",
      });
      return { method: "google", user };
    }
    return requestJson<PasswordInviteResult>("POST", API_ROUTES.users.collection, {
      email: input.email,
      role: input.role,
      method: "password",
      sendEmail: input.sendEmail,
    });
  },
  resetTemporaryPassword: (email: string, sendEmail: boolean) =>
    requestJson<PasswordInviteResult>("POST", API_ROUTES.users.temporaryPassword(email), {
      sendEmail,
    }),
  remove: (email: string) =>
    requestJson<{ ok: boolean }>("DELETE", API_ROUTES.users.item(email)),
  setRole: (email: string, role: UserRole) =>
    requestJson<User>("PUT", API_ROUTES.users.role(email), { role }),
};

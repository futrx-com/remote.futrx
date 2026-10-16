import { useCallback, useEffect, useState } from "preact/hooks";
import { userApi } from "../../../api/userApi";
import type {
  DirectoryUser,
  InviteInput,
  InviteResult,
  PasswordInviteResult,
  UserRole,
} from "../../../models/user";
import { userDirectoryState } from "./userDirectoryState";

export interface UserDirectory {
  users: DirectoryUser[] | null;
  loading: boolean;
  error: string | null;
  /** Invites an account. Credentials of a password invite are returned, never stored here. */
  add: (input: InviteInput) => Promise<InviteResult>;
  resetTemporaryPassword: (email: string, sendEmail: boolean) => Promise<PasswordInviteResult>;
  remove: (email: string) => Promise<void>;
  setRole: (email: string, role: UserRole) => Promise<void>;
}

export function useUserDirectory(enabled: boolean): UserDirectory {
  const [users, setUsers] = useState<DirectoryUser[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setUsers(await userApi.list());
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (enabled) void load();
  }, [enabled, load]);

  async function add(input: InviteInput): Promise<InviteResult> {
    const result = await userApi.add(input);
    setUsers((current) => userDirectoryState.withInvited(current, result));
    return result;
  }

  async function resetTemporaryPassword(
    email: string,
    sendEmail: boolean
  ): Promise<PasswordInviteResult> {
    const result = await userApi.resetTemporaryPassword(email, sendEmail);
    setUsers((current) => userDirectoryState.withTemporaryPassword(current, email));
    return result;
  }

  async function remove(email: string): Promise<void> {
    await userApi.remove(email);
    setUsers((current) => userDirectoryState.without(current, email));
  }

  async function setRole(email: string, role: UserRole): Promise<void> {
    await userApi.setRole(email, role);
    setUsers((current) => userDirectoryState.withRole(current, email, role));
  }

  return { users, loading, error, add, resetTemporaryPassword, remove, setRole };
}

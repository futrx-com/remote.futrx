import { useState } from "preact/hooks";
import type { InviteInput, PasswordInviteResult } from "../../../models/user";
import { userDirectoryState } from "./userDirectoryState";
import type { UserDirectory } from "./useUserDirectory";

export interface IssuedCredentials {
  /** Distinguishes one issue from the next, so a new card starts fresh. */
  serial: number;
  result: PasswordInviteResult;
}

/**
 * Holds the credentials of the latest password invite or reset for as long as
 * the card showing them is open. They live in this component state only: not
 * in the directory, a store, storage or the URL, and they go when the card is
 * dismissed or the panel unmounts.
 */
export function useIssuedCredentials(directory: Pick<UserDirectory, "add" | "resetTemporaryPassword">) {
  const [issued, setIssued] = useState<IssuedCredentials | null>(null);

  function show(result: PasswordInviteResult) {
    setIssued((current) => ({ serial: (current?.serial ?? 0) + 1, result }));
  }

  async function invite(input: InviteInput): Promise<void> {
    const credentials = userDirectoryState.issuedFrom(await directory.add(input));
    if (credentials) show(credentials);
  }

  async function reset(email: string, sendEmail: boolean): Promise<void> {
    show(await directory.resetTemporaryPassword(email, sendEmail));
  }

  return { issued, invite, reset, dismiss: () => setIssued(null) };
}

// What an extension taking over a finished attachment costs the composer.
//
// An extension claims an attachment when it is moving the file somewhere else
// — s3disk copies it onto its mounted bucket and deletes the copy Remote left
// in .uploads. The prompt names the path it hands the agent, so the composer
// cannot send until every claim has settled and the file has stopped moving.
//
// Every outcome other than "a claim resolved with a path" leaves the
// attachment alone, which is the direction that cannot break a send: the file
// is still where the composer already thinks it is.

import { ATTACHMENT_CLAIM_TIMEOUT_MS } from "../../../config/chat.ts";
import { EXTENSION_EVENTS } from "../../../config/extensions.ts";
import type { CompletedUpload } from "../../../models/extension.ts";
import { extensionEventService } from "../../../services/extensions/extensionEventService.ts";

/**
 * Announces an attachment that is on disk, and waits for whatever claimed it.
 *
 * Returns null when nothing did — the ordinary case, where the upload is
 * finished the moment it is announced — and otherwise the settlement of every
 * claim, resolving with the path the file ended up at or with null when it did
 * not move. Only a claim made synchronously from a handler is waited for.
 */
export function announceUpload(
  upload: CompletedUpload,
  timeoutMs: number = ATTACHMENT_CLAIM_TIMEOUT_MS,
): Promise<string | null> | null {
  const claims: Promise<string | void>[] = [];
  extensionEventService.emit(EXTENSION_EVENTS.uploadCompleted, {
    ...upload,
    claim: (work) => {
      claims.push(work);
    },
  });
  return claims.length > 0 ? settleClaims(claims, timeoutMs) : null;
}

async function settleClaims(
  claims: Promise<string | void>[],
  timeoutMs: number,
): Promise<string | null> {
  let expire: ReturnType<typeof setTimeout> | undefined;
  const settled = await Promise.race([
    Promise.allSettled(claims),
    new Promise<null>((resolve) => {
      expire = setTimeout(() => resolve(null), timeoutMs);
    }),
  ]);
  clearTimeout(expire);
  if (!settled) return null;

  // Later claims win over earlier ones: an extension that moved the file has
  // the last word over one that only looked at it.
  let relocated: string | null = null;
  for (const result of settled) {
    if (
      result.status === "fulfilled" &&
      typeof result.value === "string" &&
      result.value
    ) {
      relocated = result.value;
    }
  }
  return relocated;
}

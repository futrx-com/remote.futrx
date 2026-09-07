// Settlement policy for extensions that take over a finished attachment.
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

export async function settleClaims(
  claims: Promise<string | void>[],
  timeoutMs: number = ATTACHMENT_CLAIM_TIMEOUT_MS,
): Promise<string | null> {
  if (claims.length === 0) return null;

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

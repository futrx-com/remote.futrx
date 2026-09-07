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

/**
 * How long to wait for claims before giving up on them.
 *
 * A claim backed by a plugin call is already bounded by the plugin host, so a
 * well-behaved one settles far inside this. The bound is for the one that
 * does not: an extension whose promise never settles would otherwise leave
 * send disabled for the rest of the session.
 */
export const CLAIM_TIMEOUT_MS = 5 * 60_000;

export async function settleClaims(
  claims: Promise<string | void>[],
  timeoutMs: number = CLAIM_TIMEOUT_MS,
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

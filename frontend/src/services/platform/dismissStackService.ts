// Which surface an Escape belongs to.
//
// Several surfaces can be dismissible at once -- find-in-chat with a menu over
// it, a modal over the sidebar, any of them over a streaming reply -- and only
// the frontmost one should close. Listener order cannot express that: two
// handlers on the window run in the order they were registered, so the surface
// that opened *first* would win, and a surface that opens later cannot get in
// front of one that is already listening.
//
// This holds the order instead. A surface claims a place when it appears and
// releases it when it goes, and a dismissal belongs to the newest claim open.

interface DismissClaim {
  id: number;
  /** Acts only when nothing else is open; see `claim`. */
  fallback: boolean;
}

/** No claim. Held by a surface that has not registered one yet. */
export const NO_DISMISS_CLAIM = 0;

class DismissStackService {
  #claims: DismissClaim[] = [];
  #lastId = NO_DISMISS_CLAIM;

  /**
   * Take a place for a surface that is now on screen, and hand back the claim
   * to release when it leaves.
   *
   * `fallback` marks a claim that is not a surface at all -- a streaming reply
   * that Escape cancels. It waits behind every surface however long it has
   * been open, because starting a run does not put it in front of the find bar
   * that was already up.
   */
  claim({ fallback = false }: { fallback?: boolean } = {}): number {
    this.#lastId += 1;
    this.#claims.push({ id: this.#lastId, fallback });
    return this.#lastId;
  }

  /** Give up a claim. Releasing one twice, or one never taken, does nothing. */
  release(claim: number): void {
    this.#claims = this.#claims.filter((held) => held.id !== claim);
  }

  /**
   * Whether a dismissal now belongs to this claim: it is the newest surface
   * open, or -- for a fallback claim -- the newest of them with no surface
   * open at all.
   */
  owns(claim: number): boolean {
    if (claim === NO_DISMISS_CLAIM) return false;
    const surfaces = this.#claims.filter((held) => !held.fallback);
    const contenders = surfaces.length > 0 ? surfaces : this.#claims;
    return contenders[contenders.length - 1]?.id === claim;
  }
}

export const dismissStackService = new DismissStackService();

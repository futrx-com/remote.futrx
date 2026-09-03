// Client-side identifiers. Neither is a uniqueness guarantee the server would
// accept — they only have to stay distinct within one open tab.
class IdService {
  /** A short random token: attachment keys, upload name suffixes. */
  random(length = 8): string {
    return Math.random().toString(36).slice(2, 2 + length);
  }

  /** A random token behind a base-36 timestamp, so ids sort in the order they
   *  were handed out — what the prompt queue renders by. */
  timeOrdered(): string {
    return `${Date.now().toString(36)}-${this.random(6)}`;
  }
}

export const idService = new IdService();

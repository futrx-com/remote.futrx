// Guarded access to localStorage. Every read can fail (private browsing, a
// blocked store, junk left by an older build) and every write can throw, and a
// remembered preference is never worth taking the app down for — so failures
// resolve to the caller's fallback and are otherwise silent.
//
// Leaf service: it knows about the browser, never about what is being stored.
class BrowserStorageService {
  readString(key: string): string | null {
    try {
      return this.store()?.getItem(key) ?? null;
    } catch {
      return null;
    }
  }

  writeString(key: string, value: string): void {
    try {
      this.store()?.setItem(key, value);
    } catch {}
  }

  readBool(key: string): boolean {
    return this.readString(key) === "true";
  }

  writeBool(key: string, value: boolean): void {
    this.writeString(key, value ? "true" : "false");
  }

  /** Parsed JSON, or null when nothing was stored or the payload is unusable.
   *  Callers own the shape check: this only promises valid JSON came back. */
  readJson(key: string): unknown {
    const raw = this.readString(key);
    if (!raw) return null;
    try {
      return JSON.parse(raw);
    } catch {
      return null;
    }
  }

  writeJson(key: string, value: unknown): void {
    try {
      this.writeString(key, JSON.stringify(value));
    } catch {}
  }

  private store(): Storage | undefined {
    try {
      return globalThis.localStorage;
    } catch {
      return undefined;
    }
  }
}

export const browserStorageService = new BrowserStorageService();

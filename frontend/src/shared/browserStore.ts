// Guarded access to localStorage. Every read can fail (private browsing, a
// blocked store, junk left by an older build) and every write can throw, and a
// remembered preference is never worth taking the app down for — so failures
// resolve to the caller's fallback and are otherwise silent.
//
// Leaf module: it knows about the browser, never about what is being stored.

function store(): Storage | undefined {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}

export function readString(key: string): string | null {
  try {
    return store()?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

export function writeString(key: string, value: string): void {
  try {
    store()?.setItem(key, value);
  } catch {}
}

export function removeString(key: string): void {
  try {
    store()?.removeItem(key);
  } catch {}
}

export function readBool(key: string): boolean {
  return readString(key) === "true";
}

export function writeBool(key: string, value: boolean): void {
  writeString(key, value ? "true" : "false");
}

/** Parsed JSON, or null when nothing was stored or the payload is unusable.
 *  Callers own the shape check: this only promises valid JSON came back. */
export function readJson(key: string): unknown {
  const raw = readString(key);
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

export function writeJson(key: string, value: unknown): void {
  try {
    writeString(key, JSON.stringify(value));
  } catch {}
}

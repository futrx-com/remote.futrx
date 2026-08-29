// Which accounts asked for notifications in *this* browser.
//
// The browser's push subscription is not durable: a service restart, a deploy,
// or the push service rotating an endpoint can leave the device with nothing
// registered even though permission was never revoked. Remembering the opt-in
// locally is what lets the app restore the subscription silently instead of
// asking the user to allow notifications all over again.
//
// It is recorded per account because a browser subscription belongs to the
// whole origin: on a shared browser, one user's opt-in must never quietly turn
// notifications on for the next person who signs in.
//
// Accounts are stored as SHA-256 fingerprints, never as addresses: localStorage
// is readable by anyone with local access to the profile, and this list must
// not enumerate who has signed in here — the same reason the server hashes the
// filenames in its push-subscription store.
//
// Leaf module: it owns one list in the browser's store and knows nothing about
// subscriptions, the server, or who is signed in.

import { STORAGE_KEYS } from "../config/storageKeys.ts";
import { readJson, removeString, writeJson } from "./browserStore.ts";

class PushDeviceOptIn {
  /** Whether this account turned notifications on in this browser before. */
  async has(account: string): Promise<boolean> {
    const wanted = await this.#fingerprint(account);
    return wanted !== "" && this.#fingerprints().includes(wanted);
  }

  /** Records that this account wants notifications on this device. */
  async remember(account: string): Promise<void> {
    const wanted = await this.#fingerprint(account);
    if (wanted === "") return;
    const known = this.#fingerprints();
    if (known.includes(wanted)) return;
    writeJson(STORAGE_KEYS.pushOptIn, [...known, wanted]);
  }

  /** Drops the opt-in, so nothing restores a device the user turned off. */
  async forget(account: string): Promise<void> {
    const wanted = await this.#fingerprint(account);
    const kept = this.#fingerprints().filter((entry) => entry !== wanted);
    if (kept.length === 0) {
      removeString(STORAGE_KEYS.pushOptIn);
      return;
    }
    writeJson(STORAGE_KEYS.pushOptIn, kept);
  }

  /**
   * One irreversible spelling per account, so a re-typed address still matches
   * but the stored list never contains an address. "" — from a blank account
   * or a context without WebCrypto — never matches and is never stored, so an
   * unusable digest degrades to "no restore", not to plaintext.
   */
  async #fingerprint(account: string): Promise<string> {
    const normalized = account.trim().toLowerCase();
    if (normalized === "") return "";
    try {
      const digest = await crypto.subtle.digest(
        "SHA-256",
        new TextEncoder().encode(normalized)
      );
      return Array.from(new Uint8Array(digest), (byte) =>
        byte.toString(16).padStart(2, "0")
      ).join("");
    } catch {
      return "";
    }
  }

  /** The stored list, tolerating anything an older build may have left. */
  #fingerprints(): string[] {
    const stored = readJson(STORAGE_KEYS.pushOptIn);
    if (!Array.isArray(stored)) return [];
    return stored.filter((entry): entry is string => typeof entry === "string");
  }
}

export const pushDeviceOptIn = new PushDeviceOptIn();

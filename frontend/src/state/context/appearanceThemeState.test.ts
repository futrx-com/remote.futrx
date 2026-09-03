import assert from "node:assert/strict";
import test from "node:test";
import { DEFAULT_USER_SETTINGS } from "../../config/settings.ts";
import { STORAGE_KEYS } from "../../config/storageKeys.ts";
import { appearanceThemeState } from "./appearanceThemeState.ts";

/** Stands in for the browser: a theme cache plus an OS that prefers dark. */
function mountBrowser(cached: string | null, systemPrefersLight = false) {
  const store = new Map<string, string>();
  if (cached !== null) store.set(STORAGE_KEYS.themeChoice, cached);
  const root = {
    dataset: {} as Record<string, string>,
    classList: { toggle: () => {} },
    style: {} as Record<string, string>,
  };

  (globalThis as any).localStorage = {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => void store.set(key, value),
  };
  (globalThis as any).window = {
    matchMedia: () => ({ matches: systemPrefersLight }),
  };
  (globalThis as any).document = { documentElement: root, querySelector: () => null };
  (globalThis as any).getComputedStyle = () => ({ getPropertyValue: () => "" });

  return { store, root };
}

test("remembered() reads back the cached choice", () => {
  mountBrowser("light");
  assert.equal(appearanceThemeState.remembered(), "light");
});

test("remembered() falls back to system for a missing or junk cache", () => {
  mountBrowser(null);
  assert.equal(appearanceThemeState.remembered(), "system");
  mountBrowser("sepia");
  assert.equal(appearanceThemeState.remembered(), "system");
});

// The bug: a light-theme user on a dark OS saw dark until the server answered,
// because the pre-settings state used the built-in "system" default.
test("seeding from the cache keeps a light choice on a dark OS", () => {
  const { root } = mountBrowser("light");
  appearanceThemeState.apply(appearanceThemeState.remembered());
  assert.equal(root.dataset.theme, "light");
});

test("the built-in default would resolve against the OS instead", () => {
  const { root } = mountBrowser("light");
  appearanceThemeState.apply(DEFAULT_USER_SETTINGS.appearance.theme);
  assert.equal(root.dataset.theme, "dark");
});

// Applying the placeholder also cached it, so the next cold boot painted its
// very first frame wrong — the flash outlived the round-trip that caused it.
test("applying the cached choice leaves the cache intact", () => {
  const { store } = mountBrowser("light");
  appearanceThemeState.apply(appearanceThemeState.remembered());
  assert.equal(store.get(STORAGE_KEYS.themeChoice), "light");
});

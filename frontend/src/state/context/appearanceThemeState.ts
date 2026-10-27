import { SYSTEM_LIGHT_MEDIA_QUERY, VALID_APPEARANCE_THEMES } from "../../config/settings.ts";
import { STORAGE_KEYS } from "../../config/storageKeys.ts";
import { browserStorageService } from "../../services/platform/browserStorageService.ts";
import type { AppearanceTheme } from "../../models/settings";

class AppearanceThemeState {
  apply(theme: AppearanceTheme): void {
    if (typeof document === "undefined") return;
    const resolved = this.resolve(theme);
    const root = document.documentElement;
    root.dataset.theme = resolved;
    root.dataset.themeChoice = theme;
    root.classList.toggle("light", resolved === "light");
    root.classList.toggle("dark", resolved === "dark");
    root.style.colorScheme = resolved;
    this.syncBrowserChrome();
    this.remember(theme);
  }

  /**
   * The choice this browser last applied, or "system" when nothing is cached.
   *
   * The bootstrap script in index.html paints the first frame from this same
   * key, so anything that renders before the server's settings arrive has to
   * start here too. Falling back to the built-in default instead would repaint
   * a light-theme user's app in dark for the length of the round-trip.
   */
  remembered(): AppearanceTheme {
    const stored = browserStorageService.readString(STORAGE_KEYS.themeChoice);
    return VALID_APPEARANCE_THEMES.has(stored as AppearanceTheme)
      ? (stored as AppearanceTheme)
      : "system";
  }

  // The real preference lives on the server, so the first paint would flash the
  // default theme on every load. Cache the choice locally; the bootstrap script
  // in index.html reads it before the app mounts.
  private remember(theme: AppearanceTheme): void {
    browserStorageService.writeString(STORAGE_KEYS.themeChoice, theme);
  }

  // Keep the mobile browser/PWA chrome on the same ground as the app frame.
  private syncBrowserChrome(): void {
    const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
    if (!meta) return;
    const ground = getComputedStyle(document.documentElement)
      .getPropertyValue("--bg-app-rgb")
      .trim();
    if (ground) meta.content = `rgb(${ground})`;
  }

  observeSystemChanges(theme: AppearanceTheme): (() => void) | undefined {
    if (theme !== "system" || typeof window === "undefined") return;

    const query = window.matchMedia(SYSTEM_LIGHT_MEDIA_QUERY);
    const applySystemTheme = () => this.apply("system");
    query.addEventListener("change", applySystemTheme);
    return () => query.removeEventListener("change", applySystemTheme);
  }

  private resolve(theme: AppearanceTheme): "dark" | "light" {
    if (theme === "light" || theme === "dark") return theme;
    if (typeof window === "undefined") return "dark";
    return window.matchMedia(SYSTEM_LIGHT_MEDIA_QUERY).matches ? "light" : "dark";
  }
}

export const appearanceThemeState = new AppearanceThemeState();

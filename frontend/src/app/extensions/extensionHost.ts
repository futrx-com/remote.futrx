import { applicationsApi } from "../../api/applicationsApi.ts";
import { API_ROUTES } from "../../config/routes.ts";
import type { AppImage, AppUIExtension } from "../../models/application";
import type {
  ExtensionApi,
  ExtensionRegistry,
  ExtensionVisibility,
} from "../../models/extension";
import { extensionEventService } from "../../services/extensions/extensionEventService.ts";
import { extensionRegistry } from "../../state/stores/extensions/extensionStore.ts";
import { createExtensionApi } from "./extensionApi.ts";

type EntryModule = {
  default?: (api: ExtensionApi) => unknown;
  activate?: (api: ExtensionApi) => unknown;
};

/**
 * Loads an image's entry module from the catalog endpoint. It is the one place
 * the host reaches the network for code rather than data, so it is taken as a
 * dependency instead of being hard-wired into the loader below.
 */
export type LoadEntryModule = (url: string) => Promise<EntryModule>;

const importEntryModule: LoadEntryModule = (url) =>
  import(/* @vite-ignore */ url) as Promise<EntryModule>;

export class ExtensionHost {
  private readonly registry: ExtensionRegistry;
  private readonly loadEntryModule: LoadEntryModule;
  private readonly loaded = new Set<string>();
  private inFlight: Promise<void> | null = null;
  private resyncRequested = false;

  constructor(
    registry: ExtensionRegistry,
    loadEntryModule: LoadEntryModule = importEntryModule,
  ) {
    this.registry = registry;
    this.loadEntryModule = loadEntryModule;
  }

  /**
   * Syncs now, and again whenever the tab comes back to the foreground, until
   * the returned disposer is called.
   *
   * Nothing pushes installs to a browser. An app uninstalled elsewhere would
   * otherwise keep drawing here for the life of the tab, calling a plugin that
   * is no longer running, so returning to the tab is one of the few moments
   * this side can learn about it.
   */
  watch = (): (() => void) => {
    void this.sync();
    const resync = () => {
      if (document.visibilityState === "visible") void this.sync();
    };
    document.addEventListener("visibilitychange", resync);
    return () => document.removeEventListener("visibilitychange", resync);
  };

  sync = (): Promise<void> => {
    if (this.inFlight) {
      this.resyncRequested = true;
      return this.inFlight.then(() =>
        this.resyncRequested ? this.sync() : undefined,
      );
    }
    this.resyncRequested = false;
    this.inFlight = this.loadInstalled().finally(() => {
      this.inFlight = null;
    });
    return this.inFlight;
  };

  private async loadInstalled(): Promise<void> {
    let extensions: AppUIExtension[];
    try {
      extensions = (await applicationsApi.uiExtensions()) ?? [];
    } catch (error) {
      console.warn("[extensions] extension list unavailable", error);
      return;
    }

    this.removeInactive(extensions);
    for (const extension of extensions) {
      this.registry.setVisibility(
        extension.image.id,
        this.visibilityOf(extension),
      );
    }
    await Promise.all(
      extensions
        .filter((extension) => extension.image.ui)
        .map((extension) => this.loadImage(extension)),
    );
  }

  private removeInactive(extensions: AppUIExtension[]): void {
    const installed = new Set(
      extensions.map((extension) => extension.image.id),
    );
    for (const imageId of this.loaded) {
      if (installed.has(imageId)) continue;
      this.forget(imageId);
    }
  }

  private async loadImage(extension: AppUIExtension): Promise<void> {
    const { image } = extension;
    if (this.loaded.has(image.id)) return;
    this.loaded.add(image.id);
    try {
      for (const style of image.ui?.styles ?? []) {
        this.injectStylesheet(image.id, style);
      }
      const entry = image.ui?.entry;
      if (!entry) return;
      const module = await this.loadEntryModule(
        API_ROUTES.applications.uiAsset(image.id, entry),
      );
      const activate = module.default ?? module.activate;
      if (typeof activate !== "function") {
        console.warn(
          `[extensions] ${image.id}: ${entry} exports no default function`,
        );
        return;
      }
      await activate(
        createExtensionApi(
          image,
          this.visibilityOf(extension),
          extension.backends ?? [],
          this.registry,
        ),
      );
    } catch (error) {
      console.error(`[extensions] ${image.id} failed to load`, error);
      this.forget(image.id);
    }
  }

  /**
   * Drops everything an image contributed. Its entry module stays in the
   * page's module cache — nothing can evict that — so removing its slot
   * renders without its event subscriptions would leave handlers firing for
   * an app that is no longer installed.
   */
  private forget(imageId: string): void {
    this.registry.removeImage(imageId);
    extensionEventService.removeImage(imageId);
    this.loaded.delete(imageId);
  }

  private visibilityOf(extension: AppUIExtension): ExtensionVisibility {
    return {
      global: extension.global,
      projectIds: extension.projectIds ?? [],
    };
  }

  private injectStylesheet(imageId: string, assetPath: string): void {
    const href = API_ROUTES.applications.uiAsset(imageId, assetPath);
    if (document.querySelector(`link[data-extension-style="${href}"]`)) return;
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = href;
    link.dataset.extensionStyle = href;
    document.head.appendChild(link);
  }
}

export const extensionHost = new ExtensionHost(extensionRegistry);

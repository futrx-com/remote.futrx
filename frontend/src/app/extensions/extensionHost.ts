import { applicationsApi } from "../../api/applicationsApi.ts";
import { API_ROUTES } from "../../config/routes.ts";
import type { AppApplication, AppUIExtension } from "../../models/application";
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
 * Loads an application's entry module from the catalog endpoint. It is the one place
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
   * otherwise keep drawing here for the life of the tab, calling a backend that
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
        extension.application.id,
        this.visibilityOf(extension),
      );
    }
    await Promise.all(
      extensions
        .filter((extension) => extension.application.ui)
        .map((extension) => this.loadImage(extension)),
    );
  }

  private removeInactive(extensions: AppUIExtension[]): void {
    const installed = new Set(
      extensions.map((extension) => extension.application.id),
    );
    for (const applicationId of this.loaded) {
      if (installed.has(applicationId)) continue;
      this.forget(applicationId);
    }
  }

  private async loadImage(extension: AppUIExtension): Promise<void> {
    const { application } = extension;
    if (this.loaded.has(application.id)) return;
    this.loaded.add(application.id);
    try {
      for (const style of application.ui?.styles ?? []) {
        this.injectStylesheet(application.id, style);
      }
      const entry = application.ui?.entry;
      if (!entry) return;
      const module = await this.loadEntryModule(
        API_ROUTES.applications.uiAsset(application.id, entry),
      );
      const activate = module.default ?? module.activate;
      if (typeof activate !== "function") {
        console.warn(
          `[extensions] ${application.id}: ${entry} exports no default function`,
        );
        return;
      }
      await activate(
        createExtensionApi(
          application,
          this.visibilityOf(extension),
          extension.backends ?? [],
          this.registry,
        ),
      );
    } catch (error) {
      console.error(`[extensions] ${application.id} failed to load`, error);
      this.forget(application.id);
    }
  }

  /**
   * Drops everything an application contributed. Its entry module stays in the
   * page's module cache — nothing can evict that — so removing its slot
   * renders without its event subscriptions would leave handlers firing for
   * an app that is no longer installed.
   */
  private forget(applicationId: string): void {
    this.registry.removeApplication(applicationId);
    extensionEventService.removeApplication(applicationId);
    this.loaded.delete(applicationId);
  }

  private visibilityOf(extension: AppUIExtension): ExtensionVisibility {
    return {
      global: extension.global,
      projectIds: extension.projectIds ?? [],
    };
  }

  private injectStylesheet(applicationId: string, assetPath: string): void {
    const href = API_ROUTES.applications.uiAsset(applicationId, assetPath);
    if (document.querySelector(`link[data-extension-style="${href}"]`)) return;
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = href;
    link.dataset.extensionStyle = href;
    document.head.appendChild(link);
  }
}

export const extensionHost = new ExtensionHost(extensionRegistry);

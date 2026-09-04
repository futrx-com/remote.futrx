import { applicationsApi } from "../../api/applicationsApi";
import { API_ROUTES } from "../../config/routes";
import type { AppImage, AppUIExtension } from "../../models/application";
import type {
  ExtensionApi,
  ExtensionRegistry,
  ExtensionVisibility,
} from "../../models/extension";
import { extensionRegistry } from "../../state/stores/extensions/extensionStore";
import { createExtensionApi } from "./extensionApi";

type EntryModule = {
  default?: (api: ExtensionApi) => unknown;
  activate?: (api: ExtensionApi) => unknown;
};

class ExtensionHost {
  private readonly loaded = new Set<string>();
  private inFlight: Promise<void> | null = null;
  private resyncRequested = false;

  constructor(private readonly registry: ExtensionRegistry) {}

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
      this.registry.removeImage(imageId);
      this.loaded.delete(imageId);
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
      const module: EntryModule = await import(
        /* @vite-ignore */ API_ROUTES.applications.uiAsset(image.id, entry)
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
          this.registry,
        ),
      );
    } catch (error) {
      console.error(`[extensions] ${image.id} failed to load`, error);
      this.registry.removeImage(image.id);
      this.loaded.delete(image.id);
    }
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

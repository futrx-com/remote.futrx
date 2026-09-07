import {
  EXTENSION_API_VERSION,
  EXTENSION_SLOTS,
  SLOT_ICON_APPEARANCE,
} from "../../config/extensions";
import { API_ROUTES } from "../../config/routes";
import type { AppBackendInstance, AppImage } from "../../models/application";
import type {
  ExtensionApi,
  ExtensionButton,
  ExtensionIconButton,
  ExtensionRegistry,
  ExtensionSlotContext,
  ExtensionVisibility,
} from "../../models/extension";
import { createBackendApi } from "./extensionBackend";
import { subscribeExtensionEvent } from "./extensionEvents";
import { openExtensionPopup } from "./extensionPopup";

const BUTTON_BASE =
  "inline-flex items-center gap-1.5 rounded-control px-2 py-1 text-[12px] " +
  "font-medium transition active:scale-[0.97]";
const BUTTON_VARIANT = {
  ghost: "text-ink-300 hover:bg-tint-strong hover:text-ink-50",
  solid: "bg-accent-blue text-on-accent hover:opacity-90",
} as const;
const ICON_BUTTON_BASE =
  "inline-flex flex-none items-center justify-center rounded-control " +
  "text-ink-400 transition-colors hover:bg-tint-strong hover:text-ink-50 " +
  "active:scale-[0.97]";

export function createExtensionApi(
  image: AppImage,
  visibility: ExtensionVisibility,
  backends: AppBackendInstance[],
  registry: ExtensionRegistry,
): ExtensionApi {
  const views = image.ui?.views ?? {};
  const assetUrl = (assetPath: string) =>
    API_ROUTES.applications.uiAsset(image.id, assetPath);
  const viewUrl = (name: string) => {
    const relativePath = views[name];
    return relativePath ? assetUrl(relativePath) : null;
  };

  return {
    apiVersion: EXTENSION_API_VERSION,
    image: {
      id: image.id,
      name: image.name,
      version: image.version,
      icon: image.icon,
    },
    install: {
      global: visibility.global,
      projectIds: [...visibility.projectIds],
    },
    slots: EXTENSION_SLOTS,
    ui: {
      register: (slot, render, options) =>
        registry.register(image.id, slot, render, options),
      addButton: (slot, button) =>
        registry.register(
          image.id,
          slot,
          (host, context) => {
            host.appendChild(renderButton(button, context));
          },
          { order: button.order, when: button.when },
        ),
      addIconButton: (slot, button) =>
        registry.register(
          image.id,
          slot,
          (host, context) => {
            host.appendChild(renderIconButton(button, context));
          },
          { order: button.order, when: button.when },
        ),
      openPopup: openExtensionPopup,
    },
    events: {
      on: (name, handler) => subscribeExtensionEvent(image.id, name, handler),
    },
    views: {
      url: viewUrl,
      load: async (name) => {
        const url = viewUrl(name);
        if (!url) throw new Error(`unknown view "${name}"`);
        const response = await fetch(url, { credentials: "same-origin" });
        if (!response.ok) {
          throw new Error(`view "${name}" failed: ${response.status}`);
        }
        return response.text();
      },
    },
    assets: { url: assetUrl },
    backend: createBackendApi(image, backends),
    log: (...args) => console.info(`[extension:${image.id}]`, ...args),
  };
}

function renderIconButton(
  button: ExtensionIconButton,
  context: ExtensionSlotContext,
): HTMLButtonElement {
  const appearance = SLOT_ICON_APPEARANCE[context.slot];
  const element = document.createElement("button");
  element.type = "button";
  element.className = `${ICON_BUTTON_BASE} ${appearance.button}`;
  element.setAttribute("aria-label", button.label);
  element.title = button.title ?? button.label;

  const icon = document.createElement("span");
  icon.className =
    `grid place-items-center ${appearance.icon} [&>svg]:h-full [&>svg]:w-full`;
  icon.setAttribute("aria-hidden", "true");
  icon.innerHTML = button.icon;
  element.appendChild(icon);

  attachHandler(element, () => button.onClick(context));
  return element;
}

function renderButton(
  button: ExtensionButton,
  context: ExtensionSlotContext,
): HTMLButtonElement {
  const element = document.createElement("button");
  element.type = "button";
  element.className =
    `${BUTTON_BASE} ${BUTTON_VARIANT[button.variant ?? "ghost"]}`;
  if (button.title) element.title = button.title;
  if (button.icon) {
    const icon = document.createElement("span");
    icon.className = "inline-flex h-4 w-4 items-center justify-center";
    icon.innerHTML = button.icon;
    element.appendChild(icon);
  }
  element.appendChild(document.createTextNode(button.label));
  attachHandler(element, () => button.onClick(context));
  return element;
}

function attachHandler(element: HTMLElement, handler: () => void): void {
  element.addEventListener("click", (event) => {
    event.preventDefault();
    event.stopPropagation();
    try {
      handler();
    } catch (error) {
      console.error("[extensions] button handler failed", error);
    }
  });
}

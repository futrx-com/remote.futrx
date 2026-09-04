import type { AppImage, AppInstance, AppScope } from "./application";

export interface ExtensionSlotCatalog {
  sidebarHeaderActions: "sidebar.header.actions";
  sidebarSearchActions: "sidebar.search.actions";
  projectRowActions: "sidebar.project.actions";
  chatHeaderActions: "chat.header.actions";
  composerActions: "chat.composer.actions";
  applicationCardActions: "applications.card.actions";
  applicationsPanel: "applications.panel";
}

export type ExtensionSlotName =
  ExtensionSlotCatalog[keyof ExtensionSlotCatalog];

export interface ExtensionSlotContext {
  slot: ExtensionSlotName;
  scope?: AppScope;
  instance?: AppInstance;
  projectId?: string;
  projectName?: string;
  chatId?: string;
  cwd?: string;
}

export type ExtensionRender = (
  host: HTMLElement,
  context: ExtensionSlotContext,
) => void | (() => void);

export type ExtensionPredicate = (context: ExtensionSlotContext) => boolean;

export interface ExtensionVisibility {
  global: boolean;
  projectIds: string[];
}

export interface ExtensionContribution {
  id: string;
  imageId: string;
  slot: ExtensionSlotName;
  order: number;
  render: ExtensionRender;
  when?: ExtensionPredicate;
  visibility: ExtensionVisibility;
}

export interface ExtensionRegisterOptions {
  order?: number;
  when?: ExtensionPredicate;
}

export interface ExtensionRegistry {
  register: (
    imageId: string,
    slot: string,
    render: ExtensionRender,
    options?: ExtensionRegisterOptions,
  ) => () => void;
  setVisibility: (imageId: string, visibility: ExtensionVisibility) => void;
  removeImage: (imageId: string) => void;
}

export interface ExtensionStoreState {
  bySlot: ReadonlyMap<ExtensionSlotName, ExtensionContribution[]>;
  activeProjectId: string | null;
}

export interface ExtensionStoreActions extends ExtensionRegistry {
  setActiveProject: (projectId: string | null) => void;
}

export interface ExtensionButton {
  label: string;
  title?: string;
  /** Inline SVG or HTML rendered before the label. */
  icon?: string;
  onClick: (context: ExtensionSlotContext) => void;
  order?: number;
  when?: ExtensionPredicate;
  variant?: "ghost" | "solid";
}

export interface ExtensionIconButton {
  /** Inline SVG for the mark. The slot controls its dimensions. */
  icon: string;
  label: string;
  title?: string;
  onClick: (context: ExtensionSlotContext) => void;
  order?: number;
  when?: ExtensionPredicate;
}

export interface ExtensionPopupOptions {
  title?: string;
  html?: string;
  mount?: (body: HTMLElement) => void | (() => void);
  width?: number;
}

export interface ExtensionPopupHandle {
  readonly body: HTMLElement;
  close: () => void;
}

export interface ExtensionApi {
  apiVersion: number;
  image: Pick<AppImage, "id" | "name" | "version" | "icon">;
  install: ExtensionVisibility;
  slots: ExtensionSlotCatalog;
  ui: {
    register: (
      slot: string,
      render: ExtensionRender,
      options?: ExtensionRegisterOptions,
    ) => () => void;
    addButton: (slot: string, button: ExtensionButton) => () => void;
    addIconButton: (slot: string, button: ExtensionIconButton) => () => void;
    openPopup: (options?: ExtensionPopupOptions) => ExtensionPopupHandle;
  };
  views: {
    load: (name: string) => Promise<string>;
    url: (name: string) => string | null;
  };
  assets: {
    url: (assetPath: string) => string;
  };
  log: (...args: unknown[]) => void;
}

export interface SlotIconAppearance {
  button: string;
  icon: string;
}

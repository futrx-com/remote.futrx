import type {
  AppBackendDescriptor,
  AppBackendInstance,
  AppApplication,
  AppInstance,
  AppScope,
} from "./application";
import type { FileOpenRequest } from "./files";

export interface ExtensionSlotCatalog {
  sidebarHeaderActions: "sidebar.header.actions";
  sidebarSearchActions: "sidebar.search.actions";
  projectRowActions: "sidebar.project.actions";
  chatHeaderActions: "chat.header.actions";
  composerActions: "chat.composer.actions";
  applicationCardActions: "applications.card.actions";
  applicationsPanel: "applications.panel";
  projectSettingsPanel: "project.settings.panel";
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
  /** Tooltip direction supplied by the workspace toolbar layout. */
  tooltipPlacement?: "below" | "left";
}

export interface ExtensionEventCatalog {
  uploadCompleted: "upload.completed";
}

export type ExtensionEventName =
  ExtensionEventCatalog[keyof ExtensionEventCatalog];

/**
 * One finished chat attachment. The paths are the container's, because that is
 * what an extension's backend acts on and what the prompt hands the agent.
 */
export interface CompletedUpload {
  chatId: string;
  /** Set for a project chat; absent for one that is not in a project. */
  projectId?: string;
  /** The unique name the upload was stored under, not the name shown on screen. */
  fileName: string;
  /** The directory holding it, e.g. `/workspace/.uploads`. */
  directory: string;
  /** `directory/fileName`, the path the prompt gives the agent. */
  path: string;
  size: number;
}

/** The upload a handler is given, and the way it takes the attachment over. */
export interface UploadCompletedEvent extends CompletedUpload {
  /**
   * Take over the attachment. Call this synchronously from the handler with
   * the work you are starting.
   *
   * The composer will not send until every claim settles, because the prompt
   * cannot name a file that is still being moved. A claim resolving with a
   * string replaces the path the prompt gives the agent — resolve with one
   * only once the file is actually readable there. A claim that rejects, or
   * resolves with nothing, leaves the attachment exactly as it is.
   */
  claim: (work: Promise<string | void>) => void;
}

export interface ExtensionEventMap {
  "upload.completed": UploadCompletedEvent;
}

export type ExtensionEventHandler<Name extends ExtensionEventName> = (
  payload: ExtensionEventMap[Name],
) => void;

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
  applicationId: string;
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

/** The chat a drawer is docked beside. */
export interface ExtensionDrawerTarget {
  chatId: string;
  projectId?: string;
  cwd?: string;
}

/** The line under a drawer's title, and the dot beside it. */
export interface ExtensionDrawerStatus {
  label: string;
  /** Green dot when true, grey when false. */
  active: boolean;
}

export interface ExtensionDrawerContext extends ExtensionDrawerTarget {
  /** Replace the header's status line; `null` removes it. */
  setStatus: (status: ExtensionDrawerStatus | null) => void;
}

export type ExtensionDrawerPredicate = (target: ExtensionDrawerTarget) => boolean;

export interface ExtensionDrawerOptions {
  /** Lowercase letters, digits and hyphens; unique within the application. */
  id: string;
  /** Heading of the pane. */
  title: string;
  /** Name of the header toggle. Defaults to `title`. */
  label?: string;
  /** Inline SVG for the header toggle and the pane heading. */
  icon: string;
  order?: number;
  when?: ExtensionDrawerPredicate;
  /** Starting width in pixels until the user drags the pane. */
  defaultWidth?: number;
  minWidth?: number;
  /**
   * Fill the pane's body. Runs the first time the drawer is opened for a chat;
   * the body then stays mounted while the pane is closed, and the returned
   * cleanup runs when the chat changes or the drawer is removed.
   */
  mount: (body: HTMLElement, context: ExtensionDrawerContext) => void | (() => void);
}

export interface ExtensionDrawerContribution {
  /** `<applicationId>-<options.id>`, safe to use in a DOM id. */
  id: string;
  applicationId: string;
  title: string;
  label: string;
  icon: string;
  order: number;
  when?: ExtensionDrawerPredicate;
  defaultWidth: number;
  minWidth: number;
  mount: ExtensionDrawerOptions["mount"];
  visibility: ExtensionVisibility;
}

export interface ExtensionRegistry {
  register: (
    applicationId: string,
    slot: string,
    render: ExtensionRender,
    options?: ExtensionRegisterOptions,
  ) => () => void;
  registerDrawer: (
    applicationId: string,
    options: ExtensionDrawerOptions,
  ) => () => void;
  setVisibility: (applicationId: string, visibility: ExtensionVisibility) => void;
  removeApplication: (applicationId: string) => void;
}

export interface ExtensionStoreState {
  bySlot: ReadonlyMap<ExtensionSlotName, ExtensionContribution[]>;
  drawers: ExtensionDrawerContribution[];
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

/**
 * Which running backend a call should reach. An application installed in more than
 * one place runs a process per install, so a call that does not say resolves
 * to the global one.
 */
export interface ExtensionBackendTarget {
  /** Address one instance explicitly, by the id from `remote.backend.instances`. */
  instanceId?: string;
  /**
   * Prefer the instance installed in this project, falling back to the global
   * one. Pass `context.projectId` from a slot and a call follows the surface
   * the user is on.
   */
  projectId?: string;
}

export interface ExtensionBackendCallOptions extends ExtensionBackendTarget {
  /** Defaults to GET, or POST when a body is given. */
  method?: string;
  /** Sent as JSON unless it is already a string. */
  body?: unknown;
  query?: Record<string, string | number | boolean | undefined>;
  headers?: Record<string, string>;
  signal?: AbortSignal;
}

/**
 * The application's own application backend. Present on every extension; `available` is false
 * when the application ships no `backend/` directory or none of its installs are
 * running, which is the case an extension should degrade around rather than
 * throw on.
 */
export interface ExtensionBackendApi {
  available: boolean;
  /** Running backends this extension may call, in install order. */
  instances: AppBackendInstance[];
  /** The URL a call would use, for `fetch`, an `<iframe>`, or a download link. */
  url: (path: string, target?: ExtensionBackendTarget) => string;
  /** What the backend says about itself, including the routes it serves. */
  describe: (target?: ExtensionBackendTarget) => Promise<AppBackendDescriptor>;
  /** Calls a backend route and resolves its parsed JSON body. */
  call: <T = unknown>(
    path: string,
    options?: ExtensionBackendCallOptions,
  ) => Promise<T>;
  /** Calls a backend route and resolves the raw `Response`. */
  fetch: (
    path: string,
    options?: ExtensionBackendCallOptions,
  ) => Promise<Response>;
}

export interface ExtensionApi {
  apiVersion: number;
  application: Pick<AppApplication, "id" | "name" | "version" | "icon" | "web">;
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
    /**
     * Dock a resizable pane beside the chat, with its own toggle in the chat
     * header. Returns a function that removes it.
     */
    registerDrawer: (options: ExtensionDrawerOptions) => () => void;
  };
  /**
   * Subscribe to something the SPA finished doing. `on` returns an
   * unsubscribe; every subscription is also dropped when the application is
   * uninstalled, so an extension that never unsubscribes still leaves nothing
   * behind. A handler's return value is ignored and a handler that throws is
   * logged, never propagated — the SPA does not wait for extensions.
   */
  events: {
    on: <Name extends ExtensionEventName>(
      name: Name,
      handler: ExtensionEventHandler<Name>,
    ) => () => void;
  };
  views: {
    load: (name: string) => Promise<string>;
    url: (name: string) => string | null;
  };
  assets: {
    url: (assetPath: string) => string;
  };
  files: {
    registerOpener: (open: (request: FileOpenRequest) => string | null) => () => void;
  };
  backend: ExtensionBackendApi;
  log: (...args: unknown[]) => void;
}

export interface SlotIconAppearance {
  button: string;
  icon: string;
}

import type {
  AppBackendDescriptor,
  AppBackendInstance,
  AppApplication,
  AppInstance,
  AppScope,
} from "./application";

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

/** Context supplied to an application-owned pane in the active chat workspace. */
export interface ExtensionWorkspacePaneContext {
  chatId: string;
  projectId?: string;
  cwd: string;
}

export type ExtensionWorkspacePaneRender = (
  host: HTMLElement,
  context: ExtensionWorkspacePaneContext,
) => void | (() => void);

export type ExtensionWorkspacePanePredicate = (
  context: ExtensionWorkspacePaneContext,
) => boolean;

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

/** Public options accepted by `remote.ui.addWorkspacePane`. */
export interface ExtensionWorkspacePane {
  /** Stable within this application; used for diagnostics and accessibility. */
  id: string;
  /** Accessible name and tooltip for the chat-header trigger. */
  label: string;
  /** Optional trigger tooltip. The pane heading remains `label`. */
  title?: string;
  /** Inline SVG/HTML mark. Core controls its dimensions and colour. */
  icon: string;
  /** Preferred desktop width in pixels, clamped by core and the viewport. */
  width?: number;
  render: ExtensionWorkspacePaneRender;
  order?: number;
  when?: ExtensionWorkspacePanePredicate;
}

/** Internal, install-scoped form retained by the extension registry. */
export interface ExtensionWorkspacePaneContribution
  extends Omit<ExtensionWorkspacePane, "id"> {
  id: string;
  paneId: string;
  applicationId: string;
  order: number;
  visibility: ExtensionVisibility;
}

export interface ExtensionRegisterOptions {
  order?: number;
  when?: ExtensionPredicate;
}

export interface ExtensionRegistry {
  register: (
    applicationId: string,
    slot: string,
    render: ExtensionRender,
    options?: ExtensionRegisterOptions,
  ) => () => void;
  registerWorkspacePane: (
    applicationId: string,
    pane: ExtensionWorkspacePane,
  ) => () => void;
  setVisibility: (applicationId: string, visibility: ExtensionVisibility) => void;
  removeApplication: (applicationId: string) => void;
}

export interface ExtensionStoreState {
  bySlot: ReadonlyMap<ExtensionSlotName, ExtensionContribution[]>;
  workspacePanes: ExtensionWorkspacePaneContribution[];
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
   * Address the backend through an authorized chat. Remote then supplies the
   * backend with trusted chat, project, and workspace context.
   */
  chatId?: string;
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
  application: Pick<AppApplication, "id" | "name" | "version" | "icon">;
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
    addWorkspacePane: (pane: ExtensionWorkspacePane) => () => void;
    openPopup: (options?: ExtensionPopupOptions) => ExtensionPopupHandle;
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
  backend: ExtensionBackendApi;
  log: (...args: unknown[]) => void;
}

export interface SlotIconAppearance {
  button: string;
  icon: string;
}

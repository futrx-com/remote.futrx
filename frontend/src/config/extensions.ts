import type {
  ExtensionEventCatalog,
  ExtensionEventName,
  ExtensionSlotCatalog,
  ExtensionSlotName,
  ExtensionVisibility,
  SlotIconAppearance,
} from "../models/extension";

export const EXTENSION_API_VERSION = 1;

export const EXTENSION_SLOTS = {
  sidebarHeaderActions: "sidebar.header.actions",
  sidebarSearchActions: "sidebar.search.actions",
  projectRowActions: "sidebar.project.actions",
  chatHeaderActions: "chat.header.actions",
  composerActions: "chat.composer.actions",
  applicationCardActions: "applications.card.actions",
  applicationsPanel: "applications.panel",
  projectSettingsPanel: "project.settings.panel",
} as const satisfies ExtensionSlotCatalog;

export const EXTENSION_SLOT_NAMES: ExtensionSlotName[] =
  Object.values(EXTENSION_SLOTS);

export const DEFAULT_EXTENSION_VISIBILITY: ExtensionVisibility = {
  global: true,
  projectIds: [],
};

const CHROME_ICON: SlotIconAppearance = { button: "h-8 w-8", icon: "h-4 w-4" };
const COMPACT_ICON: SlotIconAppearance = {
  button: "h-7 w-7",
  icon: "h-3.5 w-3.5",
};
const INLINE_ICON: SlotIconAppearance = { button: "h-5 w-5", icon: "h-3 w-3" };

export const SLOT_ICON_APPEARANCE: Record<ExtensionSlotName, SlotIconAppearance> = {
  [EXTENSION_SLOTS.sidebarHeaderActions]: CHROME_ICON,
  [EXTENSION_SLOTS.sidebarSearchActions]: INLINE_ICON,
  [EXTENSION_SLOTS.projectRowActions]: COMPACT_ICON,
  [EXTENSION_SLOTS.chatHeaderActions]: CHROME_ICON,
  [EXTENSION_SLOTS.composerActions]: CHROME_ICON,
  [EXTENSION_SLOTS.applicationCardActions]: COMPACT_ICON,
  [EXTENSION_SLOTS.applicationsPanel]: CHROME_ICON,
  [EXTENSION_SLOTS.projectSettingsPanel]: CHROME_ICON,
};

/**
 * Events an extension may subscribe to through `remote.events.on`. They are
 * the SPA telling extensions that something finished, not a request for them
 * to do anything: an extension that handles none of them still works, and the
 * SPA does not wait for the ones that do.
 */
export const EXTENSION_EVENTS = {
  /** One chat attachment finished uploading and is on disk in the workspace. */
  uploadCompleted: "upload.completed",
} as const satisfies ExtensionEventCatalog;

export const EXTENSION_EVENT_NAMES: ExtensionEventName[] =
  Object.values(EXTENSION_EVENTS);

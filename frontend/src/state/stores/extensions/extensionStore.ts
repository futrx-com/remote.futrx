import { createStore } from "zustand/vanilla";
import {
  DEFAULT_EXTENSION_VISIBILITY,
  EXTENSION_DRAWER_ID_PATTERN,
  EXTENSION_DRAWER_WIDTH,
  EXTENSION_SLOT_NAMES,
} from "../../../config/extensions.ts";
import type {
  ExtensionContribution,
  ExtensionDrawerContribution,
  ExtensionRegisterOptions,
  ExtensionRegistry,
  ExtensionRender,
  ExtensionSlotName,
  ExtensionStoreActions,
  ExtensionStoreState,
  ExtensionVisibility,
} from "../../../models/extension.ts";

export function createExtensionStore() {
  const counters = new Map<string, number>();
  const visibilityByImage = new Map<string, ExtensionVisibility>();

  return createStore<ExtensionStoreState & ExtensionStoreActions>()((set) => ({
    bySlot: new Map(),
    drawers: [],
    activeProjectId: null,

    setVisibility: (applicationId, visibility) => {
      const current = visibilityByImage.get(applicationId);
      if (sameVisibility(current, visibility)) return;
      visibilityByImage.set(applicationId, visibility);
      set((state) => ({
        bySlot: mapContributions(state.bySlot, (contribution) =>
          contribution.applicationId === applicationId
            ? { ...contribution, visibility }
            : contribution,
        ),
        drawers: state.drawers.map((drawer) =>
          drawer.applicationId === applicationId ? { ...drawer, visibility } : drawer,
        ),
      }));
    },

    setActiveProject: (activeProjectId) => set((state) =>
      state.activeProjectId === activeProjectId ? state : { activeProjectId },
    ),

    register: (applicationId, slot, render, options = {}) => {
      if (!isExtensionSlot(slot)) {
        console.warn(`[extensions] ${applicationId}: unknown slot "${slot}"`);
        return () => {};
      }

      const sequence = (counters.get(applicationId) ?? 0) + 1;
      counters.set(applicationId, sequence);
      const contribution: ExtensionContribution = {
        id: `${applicationId}#${sequence}`,
        applicationId,
        slot,
        order: options.order ?? 0,
        render,
        when: options.when,
        visibility: visibilityByImage.get(applicationId) ?? DEFAULT_EXTENSION_VISIBILITY,
      };
      set((state) => {
        const bySlot = new Map(state.bySlot);
        const contributions = [...(bySlot.get(slot) ?? []), contribution];
        contributions.sort((left, right) => left.order - right.order);
        bySlot.set(slot, contributions);
        return { bySlot };
      });

      let disposed = false;
      return () => {
        if (disposed) return;
        disposed = true;
        set((state) => removeContribution(state, contribution));
      };
    },

    registerDrawer: (applicationId, options) => {
      if (!EXTENSION_DRAWER_ID_PATTERN.test(options.id)) {
        console.warn(`[extensions] ${applicationId}: invalid drawer id "${options.id}"`);
        return () => {};
      }
      const id = `${applicationId}-${options.id}`;
      const minWidth = Math.max(options.minWidth ?? EXTENSION_DRAWER_WIDTH.min, 1);
      const drawer: ExtensionDrawerContribution = {
        id,
        applicationId,
        title: options.title,
        label: options.label ?? options.title,
        icon: options.icon,
        order: options.order ?? 0,
        when: options.when,
        defaultWidth: Math.max(options.defaultWidth ?? EXTENSION_DRAWER_WIDTH.default, minWidth),
        minWidth,
        mount: options.mount,
        visibility: visibilityByImage.get(applicationId) ?? DEFAULT_EXTENSION_VISIBILITY,
      };
      let registered = false;
      set((state) => {
        // Two panes sharing an id would share a DOM id and a stored width.
        if (state.drawers.some((candidate) => candidate.id === id)) {
          console.warn(`[extensions] ${applicationId}: drawer "${options.id}" is already registered`);
          return state;
        }
        registered = true;
        const drawers = [...state.drawers, drawer];
        drawers.sort((left, right) => left.order - right.order);
        return { drawers };
      });
      if (!registered) return () => {};

      let disposed = false;
      return () => {
        if (disposed) return;
        disposed = true;
        set((state) => state.drawers.includes(drawer)
          ? { drawers: state.drawers.filter((candidate) => candidate !== drawer) }
          : state);
      };
    },

    removeApplication: (applicationId) => {
      visibilityByImage.delete(applicationId);
      set((state) => state.drawers.some((drawer) => drawer.applicationId === applicationId)
        ? { drawers: state.drawers.filter((drawer) => drawer.applicationId !== applicationId) }
        : state);
      set((state) => {
        const bySlot = new Map<ExtensionSlotName, ExtensionContribution[]>();
        let changed = false;
        for (const [slot, contributions] of state.bySlot) {
          const remaining = contributions.filter(
            (contribution) => contribution.applicationId !== applicationId,
          );
          changed ||= remaining.length !== contributions.length;
          if (remaining.length) bySlot.set(slot, remaining);
        }
        return changed ? { bySlot } : state;
      });
    },
  }));
}

function isExtensionSlot(slot: string): slot is ExtensionSlotName {
  return (EXTENSION_SLOT_NAMES as string[]).includes(slot);
}

function sameVisibility(
  current: ExtensionVisibility | undefined,
  next: ExtensionVisibility,
): boolean {
  return !!current
    && current.global === next.global
    && current.projectIds.length === next.projectIds.length
    && current.projectIds.every((projectId) => next.projectIds.includes(projectId));
}

function mapContributions(
  current: ReadonlyMap<ExtensionSlotName, ExtensionContribution[]>,
  map: (contribution: ExtensionContribution) => ExtensionContribution,
): ReadonlyMap<ExtensionSlotName, ExtensionContribution[]> {
  const bySlot = new Map<ExtensionSlotName, ExtensionContribution[]>();
  for (const [slot, contributions] of current) {
    bySlot.set(slot, contributions.map(map));
  }
  return bySlot;
}

function removeContribution(
  state: ExtensionStoreState,
  contribution: ExtensionContribution,
): Partial<ExtensionStoreState> | ExtensionStoreState {
  const current = state.bySlot.get(contribution.slot);
  if (!current?.includes(contribution)) return state;
  const bySlot = new Map(state.bySlot);
  const remaining = current.filter((candidate) => candidate !== contribution);
  if (remaining.length) bySlot.set(contribution.slot, remaining);
  else bySlot.delete(contribution.slot);
  return { bySlot };
}

export const extensionStore = createExtensionStore();

export const extensionRegistry: ExtensionRegistry = {
  register: (...args) => extensionStore.getState().register(...args),
  registerDrawer: (...args) => extensionStore.getState().registerDrawer(...args),
  setVisibility: (...args) => extensionStore.getState().setVisibility(...args),
  removeApplication: (...args) => extensionStore.getState().removeApplication(...args),
};

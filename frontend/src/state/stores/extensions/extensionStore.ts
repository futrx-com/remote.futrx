import { createStore } from "zustand/vanilla";
import {
  DEFAULT_EXTENSION_VISIBILITY,
  EXTENSION_SLOT_NAMES,
} from "../../../config/extensions.ts";
import type {
  ExtensionContribution,
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
    activeProjectId: null,

    setVisibility: (imageId, visibility) => {
      const current = visibilityByImage.get(imageId);
      if (sameVisibility(current, visibility)) return;
      visibilityByImage.set(imageId, visibility);
      set((state) => ({
        bySlot: mapContributions(state.bySlot, (contribution) =>
          contribution.imageId === imageId
            ? { ...contribution, visibility }
            : contribution,
        ),
      }));
    },

    setActiveProject: (activeProjectId) => set((state) =>
      state.activeProjectId === activeProjectId ? state : { activeProjectId },
    ),

    register: (imageId, slot, render, options = {}) => {
      if (!isExtensionSlot(slot)) {
        console.warn(`[extensions] ${imageId}: unknown slot "${slot}"`);
        return () => {};
      }

      const sequence = (counters.get(imageId) ?? 0) + 1;
      counters.set(imageId, sequence);
      const contribution: ExtensionContribution = {
        id: `${imageId}#${sequence}`,
        imageId,
        slot,
        order: options.order ?? 0,
        render,
        when: options.when,
        visibility: visibilityByImage.get(imageId) ?? DEFAULT_EXTENSION_VISIBILITY,
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

    removeImage: (imageId) => {
      visibilityByImage.delete(imageId);
      set((state) => {
        const bySlot = new Map<ExtensionSlotName, ExtensionContribution[]>();
        let changed = false;
        for (const [slot, contributions] of state.bySlot) {
          const remaining = contributions.filter(
            (contribution) => contribution.imageId !== imageId,
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
  setVisibility: (...args) => extensionStore.getState().setVisibility(...args),
  removeImage: (...args) => extensionStore.getState().removeImage(...args),
};

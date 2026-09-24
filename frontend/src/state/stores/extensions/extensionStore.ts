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
  ExtensionWorkspacePane,
  ExtensionWorkspacePaneContribution,
} from "../../../models/extension.ts";

export function createExtensionStore() {
  const counters = new Map<string, number>();
  const visibilityByImage = new Map<string, ExtensionVisibility>();

  return createStore<ExtensionStoreState & ExtensionStoreActions>()((set, get) => ({
    bySlot: new Map(),
    workspacePanes: [],
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
        workspacePanes: state.workspacePanes.map((pane) =>
          pane.applicationId === applicationId ? { ...pane, visibility } : pane,
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

    registerWorkspacePane: (applicationId, pane: ExtensionWorkspacePane) => {
      const paneId = typeof pane?.id === "string" ? pane.id.trim() : "";
      const label = typeof pane?.label === "string" ? pane.label.trim() : "";
      const icon = typeof pane?.icon === "string" ? pane.icon.trim() : "";
      if (
        !workspacePaneIDPattern.test(paneId)
        || !label
        || !icon
        || typeof pane.render !== "function"
      ) {
        console.warn(
          `[extensions] ${applicationId}: workspace panes require a lowercase id, label, icon, and render`,
        );
        return () => {};
      }
      const contributionId = `${applicationId}:${paneId}`;
      if (get().workspacePanes.some((candidate) => candidate.id === contributionId)) {
        console.warn(
          `[extensions] ${applicationId}: duplicate workspace pane id "${paneId}" ignored`,
        );
        return () => {};
      }
      const contribution: ExtensionWorkspacePaneContribution = {
        ...pane,
        id: contributionId,
        paneId,
        label,
        icon,
        width: normalizePaneWidth(pane.width),
        applicationId,
        order: pane.order ?? 0,
        visibility: visibilityByImage.get(applicationId) ?? DEFAULT_EXTENSION_VISIBILITY,
      };
      set((state) => ({
        workspacePanes: [...state.workspacePanes, contribution].sort(
          (left, right) => left.order - right.order,
        ),
      }));

      let disposed = false;
      return () => {
        if (disposed) return;
        disposed = true;
        set((state) => ({
          workspacePanes: state.workspacePanes.filter(
            (candidate) => candidate !== contribution,
          ),
        }));
      };
    },

    removeApplication: (applicationId) => {
      visibilityByImage.delete(applicationId);
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
        const workspacePanes = state.workspacePanes.filter(
          (pane) => pane.applicationId !== applicationId,
        );
        changed ||= workspacePanes.length !== state.workspacePanes.length;
        return changed ? { bySlot, workspacePanes } : state;
      });
    },
  }));
}

const workspacePaneIDPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

function normalizePaneWidth(width: number | undefined): number | undefined {
  if (width === undefined) return undefined;
  if (!Number.isFinite(width)) return undefined;
  return Math.min(1200, Math.max(320, Math.round(width)));
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
  registerWorkspacePane: (...args) =>
    extensionStore.getState().registerWorkspacePane(...args),
  setVisibility: (...args) => extensionStore.getState().setVisibility(...args),
  removeApplication: (...args) => extensionStore.getState().removeApplication(...args),
};

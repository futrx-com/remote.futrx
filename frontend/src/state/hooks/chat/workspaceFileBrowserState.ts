import type {
  WorkspaceFileBrowserAction,
  WorkspaceFileBrowserState,
} from "../../../models/files";

class WorkspaceFileBrowserStateTransitions {
  createInitial(): WorkspaceFileBrowserState {
    return {
      childrenByDir: new Map(),
      expanded: new Set(),
      loading: new Set(),
      errorByDir: new Map(),
      truncatedDirs: new Set(),
      rootLoading: false,
      query: "",
      searchResults: null,
      searchTruncated: false,
      searching: false,
      searchError: null,
    };
  }

  readonly reduce = (
    state: WorkspaceFileBrowserState,
    action: WorkspaceFileBrowserAction
  ): WorkspaceFileBrowserState => {
    switch (action.type) {
      case "reset":
        return this.createInitial();
      case "directory-load-started": {
        const loading = new Set(state.loading);
        loading.add(action.path);
        return {
          ...state,
          loading,
          rootLoading: action.path === "" ? true : state.rootLoading,
        };
      }
      case "directory-load-succeeded": {
        const childrenByDir = new Map(state.childrenByDir);
        childrenByDir.set(action.path, action.entries);
        const errorByDir = new Map(state.errorByDir);
        errorByDir.delete(action.path);
        const truncatedDirs = new Set(state.truncatedDirs);
        if (action.truncated) truncatedDirs.add(action.path);
        else truncatedDirs.delete(action.path);
        const loading = new Set(state.loading);
        loading.delete(action.path);
        return {
          ...state,
          childrenByDir,
          errorByDir,
          truncatedDirs,
          loading,
          rootLoading: action.path === "" ? false : state.rootLoading,
        };
      }
      case "directory-load-failed": {
        const errorByDir = new Map(state.errorByDir);
        errorByDir.set(action.path, action.error);
        const loading = new Set(state.loading);
        loading.delete(action.path);
        return {
          ...state,
          errorByDir,
          loading,
          rootLoading: action.path === "" ? false : state.rootLoading,
        };
      }
      case "directory-toggled": {
        const expanded = new Set(state.expanded);
        if (expanded.has(action.path)) expanded.delete(action.path);
        else expanded.add(action.path);
        return { ...state, expanded };
      }
      case "query-changed":
        return { ...state, query: action.query };
      case "search-idle":
        return { ...state, searchResults: null, searchError: null, searching: false };
      case "search-started":
        return { ...state, searching: true };
      case "search-succeeded":
        return {
          ...state,
          searchResults: action.entries,
          searchTruncated: action.truncated,
          searchError: null,
          searching: false,
        };
      case "search-failed":
        return {
          ...state,
          searchResults: [],
          searchError: action.error,
          searching: false,
        };
    }
  };
}

export const workspaceFileBrowserState = new WorkspaceFileBrowserStateTransitions();

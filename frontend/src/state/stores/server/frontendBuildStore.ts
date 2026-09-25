// Which frontend build the server serves, as this page last heard it, and
// what is holding the page back from reloading onto it.
//
// The answer is shared rather than kept inside the hook that reloads the page:
// the updates screen is first to hear the restarted backend answer, and asking
// from there saves the admin waiting out the next minute's check. Holds are
// shared for the same reason — the uploads they protect live far from the app
// root.

import { createStore } from "zustand/vanilla";
import { frontendBuildApi } from "../../../api/frontendBuildApi.ts";
import type {
  FrontendBuildStoreActions,
  FrontendBuildStoreState,
} from "../../../models/frontendBuild";

export function createFrontendBuildStore(
  fetchServed: () => Promise<string | null> = frontendBuildApi.served,
) {
  return createStore<FrontendBuildStoreState & FrontendBuildStoreActions>()((set) => {
    let request: Promise<void> | null = null;

    return {
      served: null,
      holds: 0,
      check: () => {
        request ??= fetchServed()
          .then((served) => {
            if (served) set({ served });
          })
          // A restarting or unreachable server says nothing about the build.
          .catch(() => {})
          .finally(() => {
            request = null;
          });
        return request;
      },
      hold: () => {
        set((state) => ({ holds: state.holds + 1 }));
        let released = false;
        return () => {
          if (released) return;
          released = true;
          set((state) => ({ holds: state.holds - 1 }));
        };
      },
    };
  });
}

export const frontendBuildStore = createFrontendBuildStore();

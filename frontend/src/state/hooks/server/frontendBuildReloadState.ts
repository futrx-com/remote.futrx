import type { FrontendBuildPage } from "../../../models/frontendBuild";

// When a page that is running an older frontend moves itself onto the one the
// server now serves. Kept apart from the hook so the rule can be read and
// pinned without a document, timers, or a server.
class FrontendBuildReloadState {
  shouldReload(page: FrontendBuildPage): boolean {
    if (!page.running || !page.served || page.served === page.running) return false;
    // This tab already reloaded for this build and still came back with an
    // older page — a cache between it and the server is answering. Reloading
    // again would loop, so stay until the server moves to yet another build.
    return page.reloadedFor !== page.served;
  }
}

export const frontendBuildReloadState = new FrontendBuildReloadState();

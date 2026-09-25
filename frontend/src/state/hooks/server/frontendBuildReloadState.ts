import type { FrontendBuildPage, FrontendBuildReload } from "../../../models/frontendBuild";

// When a page that is running an older frontend moves itself onto the one the
// server now serves. Kept apart from the hook so the rules can be read and
// pinned without a document, timers, or a server.
class FrontendBuildReloadState {
  decide(page: FrontendBuildPage): FrontendBuildReload {
    if (!page.running || !page.served || page.served === page.running) return "stay";
    // This tab already reloaded for this build and still came back with an
    // older page — a cache between it and the server is answering. Reloading
    // again would loop, so stay until the server moves to yet another build.
    if (page.reloadedFor === page.served) return "stay";
    // Uploads and the like live only in this page; a reload would drop them.
    if (page.held) return "wait";
    // Nobody is looking, or the user is only now opening the app. Drafts and
    // queued prompts survive a reload, so a focused composer is no reason to
    // leave them on the old build.
    if (page.hidden || page.opening) return "reload";
    // Mid-sentence or mid-form: pulling the page away is worse than a few more
    // minutes on the old build. The next blur, hide, or return picks it up.
    return page.editing ? "wait" : "reload";
  }
}

export const frontendBuildReloadState = new FrontendBuildReloadState();

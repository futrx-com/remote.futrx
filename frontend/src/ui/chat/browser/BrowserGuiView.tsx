import type { RefObject } from "preact";
import type { AgentBrowserStatus } from "../../../models/project";

// BrowserGuiView renders the Agent Browser pane: a status placeholder while the
// in-container session starts, then the live noVNC view as an iframe. The view
// loads from the dev-URL proxy (behind the platform's Google auth), same as the
// app preview.
export function BrowserGuiView({
  status,
  url,
  error,
  reloadKey,
  projectName,
  resizing,
  iframeRef,
}: {
  status: AgentBrowserStatus;
  url: string;
  error: string | null;
  reloadKey: number;
  projectName: string;
  resizing: boolean;
  iframeRef: RefObject<HTMLIFrameElement>;
}) {
  if (status === "ready" && url) {
    return (
      <div class="flex-1 min-h-0 bg-white">
        <iframe
          ref={iframeRef}
          key={`gui:${url}:${reloadKey}`}
          src={url}
          title={`Agent browser for ${projectName || "container"}`}
          class={`h-full w-full border-0 bg-white ${resizing ? "pointer-events-none" : ""}`}
          allow="clipboard-read; clipboard-write"
        />
      </div>
    );
  }

  return (
    <div class="flex-1 min-h-0 grid place-items-center bg-surface px-6 text-center">
      <div class="max-w-sm text-[13px] leading-relaxed text-ink-300">
        {status === "error" ? (
          <p class="text-ink-200">{error || "Couldn't start the agent browser."}</p>
        ) : status === "stopped" ? (
          <p>Agent browser stopped. Toggle it on again to restart.</p>
        ) : (
          <p>Starting the agent browser… log in to your site once it loads; the agent shares this session.</p>
        )}
      </div>
    </div>
  );
}

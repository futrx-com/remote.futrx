import type { MediaViewerItem } from "../../../models/files";
import { useMediaViewer } from "../../../state/hooks/chat/useMediaViewer";
import { useDismissShortcut } from "../../../state/hooks/shared/useDismissShortcut.ts";
import { Download, ExternalLink, X } from "../../primitives/icons";

// Full-screen host for the in-app media viewer. Mounted once per chat view;
// renders whatever mediaViewerStore currently holds.
export function MediaViewerOverlay() {
  const { item, close } = useMediaViewer();

  useDismissShortcut(close, { enabled: item !== null });

  if (!item) return null;

  return (
    <div
      class="fixed inset-0 z-[70] bg-black/75 backdrop-blur-sm flex flex-col"
      role="dialog"
      aria-modal="true"
      aria-label={item.name}
      onClick={() => close()}
    >
      <header
        class="flex-none flex items-center gap-2 px-3 md:px-4 py-2.5 bg-surface/90 border-b border-line"
        onClick={(event) => event.stopPropagation()}
      >
        <div class="min-w-0 flex-1 truncate text-[13.5px] font-medium text-ink-50" title={item.name}>
          {item.name}
        </div>
        <a
          href={item.url}
          target="_blank"
          rel="noopener noreferrer"
          class="h-9 w-9 rounded-md bg-tint hover:bg-tint-strong border border-line text-ink-200 grid place-items-center"
          title="Open in new tab"
          aria-label="Open media in new tab"
        >
          <ExternalLink class="w-4 h-4" />
        </a>
        <a
          href={item.url}
          download={item.name}
          class="h-9 w-9 rounded-md bg-tint hover:bg-tint-strong border border-line text-ink-200 grid place-items-center"
          title={`Download ${item.name}`}
          aria-label={`Download ${item.name}`}
        >
          <Download class="w-4 h-4" />
        </a>
        <button
          type="button"
          onClick={() => close()}
          class="h-9 w-9 rounded-md bg-tint hover:bg-tint-strong border border-line text-ink-200 grid place-items-center"
          title="Close viewer"
          aria-label="Close media viewer"
        >
          <X class="w-4 h-4" />
        </button>
      </header>

      <div class="flex-1 min-h-0 grid place-items-center p-4">
        <MediaContent item={item} />
      </div>
    </div>
  );
}

function MediaContent({ item }: { item: MediaViewerItem }) {
  const stop = (event: Event) => event.stopPropagation();
  switch (item.kind) {
    case "image":
      return (
        <img
          src={item.url}
          alt={item.name}
          class="max-w-full max-h-full object-contain rounded-md shadow-2xl"
          onClick={stop}
        />
      );
    case "video":
      return (
        <video
          src={item.url}
          controls
          autoPlay
          class="max-w-full max-h-full rounded-md shadow-2xl bg-black"
          onClick={stop}
        />
      );
    case "audio":
      return (
        <div
          class="w-[440px] max-w-[86vw] rounded-lg border border-line bg-surface p-4 shadow-2xl"
          onClick={stop}
        >
          <div class="mb-3 truncate text-[13px] text-ink-100" title={item.name}>{item.name}</div>
          <audio src={item.url} controls autoPlay class="w-full" />
        </div>
      );
    case "pdf":
      return (
        <iframe
          src={item.url}
          title={item.name}
          class="w-[920px] max-w-[92vw] h-full min-h-0 rounded-md bg-white shadow-2xl"
          onClick={stop}
        />
      );
  }
}

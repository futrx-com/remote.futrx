import type { ComponentChildren } from "preact";
import type { MediaKind } from "../../../models/files";
import { mediaViewerStore } from "../../../state/stores/media/mediaViewerStore";
import { fileService } from "../../../services/files/fileService.ts";
import { API_ROUTES, isChatMediaOpenUrl } from "../../../config/routes";
import { internalPathOpenUrl } from "../ideLinks";
import { File as FileIcon } from "../../primitives/icons";

// Renders the file attachments that the chat composer tucked into a user
// message. Images render as thumbnails and other viewable media (video, audio,
// PDF) as chips; both open the in-app media viewer. The rest render as
// IDE-link chips that open the file in code.
export function AttachmentPreviews({
  paths,
  chatId,
  cwd,
}: {
  paths: string[];
  chatId?: string;
  cwd?: string;
}) {
  if (!paths.length) return null;
  return (
    <div class="flex max-w-full flex-wrap gap-2 mt-1" data-testid="attachment-previews">
      {paths.map((path, index) => (
        <AttachmentPreview
          key={`${path}-${index}`}
          path={path}
          chatId={chatId}
          cwd={cwd}
        />
      ))}
    </div>
  );
}

function AttachmentPreview({
  path,
  chatId,
  cwd,
}: {
  path: string;
  chatId?: string;
  cwd?: string;
}) {
  const name = path.split("/").pop() || path;
  const mediaKind = chatId ? fileService.viewableMediaKind(name) : null;
  if (mediaKind === "image" && chatId) {
    return <AttachmentImage path={path} name={name} chatId={chatId} />;
  }
  return <AttachmentFile path={path} name={name} chatId={chatId} cwd={cwd} mediaKind={mediaKind} />;
}

function AttachmentImage({
  path,
  name,
  chatId,
}: {
  path: string;
  name: string;
  chatId: string;
}) {
  // Chat-scoped media-open URL; clicking opens the in-app viewer with the
  // original filename as the label so the viewer header reads it correctly.
  const url = API_ROUTES.chats.mediaOpen(chatId, path);
  const kind: MediaKind = "image";
  return (
    <button
      type="button"
      onClick={(event) => {
        event.preventDefault();
        mediaViewerStore.getState().open({ url, name, kind });
      }}
      class="relative w-20 h-20 rounded-lg overflow-hidden bg-surface border border-line group hover:ring-2 hover:ring-accent-blue/55 transition-shadow"
      title={path}
      aria-label={`Open ${name} in viewer`}
    >
      <img
        src={url}
        alt={name}
        loading="lazy"
        class="w-full h-full object-cover"
      />
      <div class="absolute bottom-0 left-0 right-0 px-1.5 py-0.5 bg-gradient-to-t from-black/85 to-transparent text-white text-[9.5px] truncate">
        {name}
      </div>
    </button>
  );
}

function AttachmentFile({
  path,
  name,
  chatId,
  cwd,
  mediaKind,
}: {
  path: string;
  name: string;
  chatId?: string;
  cwd?: string;
  mediaKind: MediaKind | null;
}) {
  // Picks the IDE open URL when the path is inside a workspace; otherwise
  // shows the bare path as a label.
  const internalUrl = internalPathOpenUrl(path, { chatId, cwd });
  const label: ComponentChildren = (
    <span class="flex items-center gap-1.5 min-w-0">
      <FileIcon class="w-3.5 h-3.5 text-accent-blue flex-none" />
      <span class="truncate max-w-[180px] text-ink-100">{name}</span>
    </span>
  );
  if (internalUrl) {
    return (
      <a
        href={internalUrl}
        onClick={(event) => {
          // Viewable media plays in the in-app viewer; modified clicks keep
          // the browser's default so the file can still open in a new tab.
          if (!mediaKind || !isChatMediaOpenUrl(internalUrl)) return;
          if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
          event.preventDefault();
          mediaViewerStore.getState().open({ url: internalUrl, name, kind: mediaKind });
        }}
        class="group flex items-center gap-1.5 bg-surface border border-line rounded-md px-2 py-1.5 text-xs min-h-10 hover:bg-tint-strong transition-colors"
        title={path}
      >
        {label}
      </a>
    );
  }
  return (
    <div class="flex items-center gap-1.5 bg-surface border border-line rounded-md px-2 py-1.5 text-xs min-h-10" title={path}>
      {label}
    </div>
  );
}

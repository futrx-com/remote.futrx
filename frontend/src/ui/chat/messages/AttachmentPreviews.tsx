import type { ComponentChildren } from "preact";
import type { MediaKind } from "../../../models/files";
import { mediaViewerStore } from "../../../state/stores/media/mediaViewerStore";
import { fileService } from "../../../services/files/fileService.ts";
import { internalPathOpenUrl } from "../ideLinks";
import { File as FileIcon } from "../../primitives/icons";

// Renders the file attachments that the chat composer tucked into a user
// message. Image kinds render as clickable thumbnails (open the in-app media
// viewer) and the rest render as IDE-link chips that open the file in code.
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
  if (mediaKind === "image") {
    return <AttachmentImage path={path} name={name} chatId={chatId} />;
  }
  return <AttachmentFile path={path} name={name} chatId={chatId} cwd={cwd} />;
}

function AttachmentImage({
  path,
  name,
  chatId,
}: {
  path: string;
  name: string;
  chatId?: string;
}) {
  // Chat-scoped media-open URL; clicking opens the in-app viewer with the
  // original filename as the label so the viewer header reads it correctly.
  const params = new URLSearchParams({ path });
  const url = chatId
    ? `/api/chats/${encodeURIComponent(chatId)}/media-open?${params.toString()}`
    : null;
  if (!url) {
    return <AttachmentFile path={path} name={name} chatId={undefined} cwd={undefined} />;
  }
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
}: {
  path: string;
  name: string;
  chatId?: string;
  cwd?: string;
}) {
  // Picks the IDE open URL when the path is inside a workspace; otherwise
  // shows the bare path as a label. Media paths funnel through the same
  // link, but the chat-scoped media-open URL is preferred so the viewer gets
  // a free back/forward and we do not pop a new tab.
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

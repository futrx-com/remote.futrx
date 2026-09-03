import type { Attachment } from "../../../models/upload";
import { fileService } from "../../../services/files/fileService.ts";
import { AlertCircle, File as FileIcon, X } from "../../primitives/icons";

export function AttachmentChip({
  attachment,
  onRemove,
}: {
  attachment: Attachment;
  onRemove: () => void;
}) {
  const failed = !!attachment.error;
  const pending = !attachment.serverPath && !failed;
  const pct =
    attachment.progress != null
      ? Math.max(0, Math.min(1, attachment.progress))
      : 0;
  const pctLabel = pending ? `${Math.round(pct * 100)}%` : null;
  const title = failed ? attachment.error : attachment.serverPath || attachment.name;

  if (attachment.isImage && attachment.objectUrl) {
    return (
      <div
        class="relative w-20 h-20 rounded-lg overflow-hidden bg-surface border border-line group"
        title={title}
      >
        <img
          src={attachment.objectUrl}
          class="w-full h-full object-cover"
          alt={attachment.name}
        />
        {pending && (
          <div class="absolute inset-0 bg-black/55 grid place-items-center text-white text-[11px] font-medium">
            {pctLabel}
          </div>
        )}
        {failed && (
          <div class="absolute inset-0 bg-accent-red/35 grid place-items-center">
            <AlertCircle class="w-5 h-5 text-white" />
          </div>
        )}
        <button
          type="button"
          onClick={onRemove}
          class="absolute top-1 right-1 w-6 h-6 rounded-md bg-black/70 hover:bg-accent-red text-white grid place-items-center opacity-100 md:opacity-0 md:group-hover:opacity-100 transition-opacity"
          aria-label={`Remove ${attachment.name}`}
        >
          <X class="w-3 h-3" />
        </button>
        {pending && (
          <div class="absolute bottom-0 left-0 right-0 h-1 bg-black/40">
            <div
              class="h-full bg-accent-blue transition-[width] duration-100"
              style={{ width: `${pct * 100}%` }}
            />
          </div>
        )}
        <div class="absolute bottom-0 left-0 right-0 px-1.5 py-0.5 bg-gradient-to-t from-black/85 to-transparent text-white text-[9.5px] truncate">
          {attachment.name}
        </div>
      </div>
    );
  }

  return (
    <div
      class={`group relative flex items-center gap-1.5 bg-surface border rounded-md px-2 py-1.5 text-xs min-h-10 overflow-hidden ${
        failed ? "border-accent-red/40" : "border-line"
      }`}
      title={title}
    >
      {failed ? (
        <AlertCircle class="w-3.5 h-3.5 text-accent-red flex-none" />
      ) : pending ? (
        <div class="w-3.5 h-3.5 border-2 border-ink-300 border-t-transparent rounded-full animate-spin flex-none" />
      ) : (
        <FileIcon class="w-3.5 h-3.5 text-accent-blue flex-none" />
      )}
      <span class="truncate max-w-[180px] text-ink-100">{attachment.name}</span>
      <span class="text-ink-300 text-[10px] flex-none">
        {pending && pctLabel ? pctLabel : fileService.formatBytesCompact(attachment.size)}
      </span>
      <button
        type="button"
        onClick={onRemove}
        class="w-6 h-6 grid place-items-center rounded text-ink-300 hover:text-accent-red hover:bg-accent-red/10 flex-none -mr-1"
        aria-label={`Remove ${attachment.name}`}
      >
        <X class="w-3 h-3" />
      </button>
      {pending && (
        <div class="absolute bottom-0 left-0 right-0 h-0.5 bg-tint">
          <div
            class="h-full bg-accent-blue transition-[width] duration-100"
            style={{ width: `${pct * 100}%` }}
          />
        </div>
      )}
    </div>
  );
}

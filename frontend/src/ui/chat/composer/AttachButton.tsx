import type { RefObject } from "preact";
import { Plus } from "../../primitives/icons";

export function AttachButton({
  fileInputRef,
  uploading,
  disconnected,
  onFilesSelected,
}: {
  fileInputRef: RefObject<HTMLInputElement>;
  uploading: boolean;
  disconnected: boolean;
  onFilesSelected: (files: File[]) => void;
}) {
  return (
    <>
      <button
        type="button"
        onClick={() => fileInputRef.current?.click()}
        disabled={uploading || disconnected}
        class="codex-icon-button grid h-8 w-8 flex-none place-items-center rounded-control
               text-ink-400 transition-colors hover:bg-tint-strong hover:text-ink-100
               active:scale-[0.97] disabled:opacity-40 disabled:active:scale-100"
        aria-label="Attach files"
        title="Attach (or drag-and-drop / paste images)"
      >
        <Plus class="w-4 h-4" />
      </button>
      <input
        ref={fileInputRef}
        type="file"
        multiple
        class="hidden"
        onChange={(event) => {
          const input = event.currentTarget as HTMLInputElement;
          const files = Array.from(input.files || []);
          input.value = "";
          onFilesSelected(files);
        }}
      />
    </>
  );
}

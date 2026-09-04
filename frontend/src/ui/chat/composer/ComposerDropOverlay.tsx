import { Upload } from "../../primitives/icons";

export function ComposerDropOverlay() {
  return (
    <div class="absolute inset-x-3 -top-16 z-20 rounded-lg border-2 border-dashed border-accent-blue
                bg-raised text-accent-blue text-sm flex items-center justify-center
                h-14 gap-2">
      <Upload class="w-5 h-5" />
      Drop files to upload to the chat directory
    </div>
  );
}

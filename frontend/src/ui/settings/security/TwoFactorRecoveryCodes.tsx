import type { RecoveryCodeFileFormat } from "../../../services/auth/recoveryCodeFileService";
import { recoveryCodeFileService } from "../../../services/auth/recoveryCodeFileService";
import { fileDownloadService } from "../../../services/platform/fileDownloadService";
import { Download, FileText } from "../../primitives/icons";

export function TwoFactorRecoveryCodes({
  codes,
  onDismiss,
}: {
  codes: string[];
  onDismiss: () => void;
}) {
  // The codes are shown exactly once, so the panel offers to save them. Both
  // files are built from what is already on screen - nothing is re-fetched.
  function download(format: RecoveryCodeFileFormat) {
    const file = recoveryCodeFileService.build(codes, format, new Date());
    fileDownloadService.save(file.content, file.filename, file.mimeType);
  }

  return (
    <div class="rounded-md border border-accent-yellow/30 bg-accent-yellow/[0.08] p-3 space-y-2">
      <div class="text-[13px] font-medium text-ink-50">
        Save these recovery codes now — they won't be shown again.
      </div>
      <div class="grid grid-cols-2 gap-1.5 font-mono text-[12.5px] text-ink-100">
        {codes.map((code) => (
          <div key={code} class="bg-black/30 border border-white/10 rounded px-2 py-1">
            {code}
          </div>
        ))}
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={() => download("pdf")}
          class="h-8 px-2.5 rounded bg-white/[0.08] hover:bg-white/[0.12] text-ink-100 text-[12px] font-medium inline-flex items-center gap-1.5"
        >
          <Download class="w-3.5 h-3.5" /> Download PDF
        </button>
        <button
          type="button"
          onClick={() => download("txt")}
          class="h-8 px-2.5 rounded bg-white/[0.08] hover:bg-white/[0.12] text-ink-100 text-[12px] font-medium inline-flex items-center gap-1.5"
        >
          <FileText class="w-3.5 h-3.5" /> Download .txt
        </button>
        <button
          type="button"
          onClick={onDismiss}
          class="h-8 px-2.5 rounded bg-white/[0.08] hover:bg-white/[0.12] text-ink-100 text-[12px] font-medium"
        >
          I've saved these codes
        </button>
      </div>
    </div>
  );
}

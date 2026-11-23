import { useCopyToClipboard } from "../../../state/hooks/shared/useCopyToClipboard";
import { Check, Copy } from "../../primitives/icons";

// Copies the raw block text, not the highlighted DOM, so the clipboard gets
// exactly what the agent wrote.
export function CopyCodeButton({ text }: { text: string }) {
  const { copied, copy } = useCopyToClipboard();
  const label = copied ? "Copied" : "Copy code";
  return (
    <button
      type="button"
      onClick={() => void copy(text)}
      class="absolute top-1 right-1 h-7 w-7 rounded-md grid place-items-center
             text-ink-300 bg-surface/80 hover:text-ink-100 hover:bg-tint-strong
             transition-opacity md:opacity-0 md:group-hover:opacity-100 md:focus-visible:opacity-100"
      aria-label={label}
      title={label}
    >
      {copied ? <Check class="w-3.5 h-3.5 text-accent-green" /> : <Copy class="w-3.5 h-3.5" />}
    </button>
  );
}

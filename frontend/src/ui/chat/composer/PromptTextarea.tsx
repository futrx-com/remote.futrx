import type { RefObject } from "preact";

export function PromptTextarea({
  textareaRef,
  text,
  uploading,
  streaming,
  disconnected,
  onTextChange,
  onPaste,
  onSend,
  onKeyDown,
}: {
  textareaRef: RefObject<HTMLTextAreaElement>;
  text: string;
  uploading: boolean;
  streaming: boolean;
  disconnected: boolean;
  onTextChange: (text: string) => void;
  onPaste: (event: ClipboardEvent) => void;
  onSend: () => void;
  // Return true to signal the key was handled (e.g. by the slash palette) so
  // the default composer handling below is skipped.
  onKeyDown?: (event: KeyboardEvent) => boolean;
}) {
  return (
    <textarea
      ref={textareaRef}
      dir="auto"
      value={text}
      onInput={(event) => onTextChange((event.currentTarget as HTMLTextAreaElement).value)}
      onKeyDown={(event) => {
        if (onKeyDown?.(event)) return;
        if (
          event.key === "Enter" &&
          (event.ctrlKey || event.metaKey) &&
          !event.shiftKey &&
          !event.isComposing
        ) {
          event.preventDefault();
          onSend();
        }
      }}
      onPaste={(event) => onPaste(event as ClipboardEvent)}
      rows={1}
      enterkeyhint="enter"
      aria-keyshortcuts="Control+Enter Meta+Enter"
      autocomplete="off"
      autocapitalize="off"
      autocorrect="off"
      spellcheck={false}
      placeholder={
        uploading ? "Uploading..." :
        streaming ? "Queue next prompt while the agent is working" :
        disconnected ? "Connecting..." :
        "Ask anything, @ to add files, / for commands"
      }
      disabled={disconnected}
      class="codex-composer-textarea w-full resize-none border-0 bg-transparent px-1.5 py-1.5
             text-[16px] leading-relaxed text-ink-100 placeholder:text-ink-400
             focus:outline-none disabled:opacity-60
             min-h-[44px] max-h-[220px] sm:text-[14.5px]"
    />
  );
}

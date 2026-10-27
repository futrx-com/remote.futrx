import { useMemo, useRef } from "preact/hooks";
import { MarkdownBlocks } from "../markdown/Markdown";
import { parseStreamingMarkdown } from "../markdown/blockParser";

interface BlockStreamingTextProps {
  text: string;
  streaming: boolean;
  chatId?: string;
  cwd?: string;
}

export function BlockStreamingText({ text, streaming, chatId, cwd }: BlockStreamingTextProps) {
  const blocks = useMemo(() => parseStreamingMarkdown(text, !streaming), [text, streaming]);
  const hasStreamed = useRef(streaming);
  if (streaming) hasStreamed.current = true;
  return <MarkdownBlocks blocks={blocks} chatId={chatId} cwd={cwd} streaming={hasStreamed.current} />;
}

import { useMemo } from "preact/hooks";
import { useStreamingTextState } from "../../../state/hooks/chat/useStreamingTextState";
import { MarkdownBlocks } from "../markdown/Markdown";
import { parseMarkdown } from "../markdown/blockParser";
import { BlockStreamingText } from "./BlockStreamingText";
import { TokenStreamingText } from "./TokenStreamingText";

interface Props {
  text: string;
  streaming: boolean;
  chatId?: string;
  cwd?: string;
  presentation?: "blocks" | "tokens";
  hydrated?: boolean;
}

export function StreamingText(props: Props) {
  const state = useStreamingTextState(props);
  if (state.hydrated) return <HydratedText {...props} />;

  const currentProps = { ...props, streaming: state.active };
  return state.presentation === "blocks"
    ? <BlockStreamingText {...currentProps} />
    : <TokenStreamingText {...currentProps} />;
}

function HydratedText({ text, chatId, cwd }: Props) {
  const blocks = useMemo(() => parseMarkdown(text), [text]);
  return <MarkdownBlocks blocks={blocks} chatId={chatId} cwd={cwd} streaming={false} />;
}

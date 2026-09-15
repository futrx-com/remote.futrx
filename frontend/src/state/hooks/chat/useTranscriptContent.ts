import { useState } from "preact/hooks";
import { fetchFullTranscriptContent } from "../../../api/chat/chatTranscriptApi";
import { fullResponseErrorMessage, fullResponseLabel } from "./transcriptContentPresentation";

/** Expansion belongs to the mounted detail view, including across prop updates. */
export function useTranscriptContent({
  chatId,
  content,
  contentRef,
  contentBytes,
  inlinePreviewLimit,
}: {
  chatId?: string;
  content?: string;
  contentRef?: string;
  contentBytes?: number;
  inlinePreviewLimit?: number | null;
}) {
  const [fullContent, setFullContent] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const canExpandInline = !contentRef
    && !!content
    && inlinePreviewLimit != null
    && content.length > inlinePreviewLimit;

  async function load() {
    if (loading) return;
    if (!contentRef) {
      if (content) setFullContent(content);
      return;
    }
    if (!chatId) return;
    setLoading(true);
    setError(null);
    try {
      setFullContent(await fetchFullTranscriptContent(chatId, contentRef));
    } catch (cause) {
      setError(fullResponseErrorMessage(cause));
    } finally {
      setLoading(false);
    }
  }

  return {
    content: fullContent ?? content,
    expanded: fullContent !== null,
    canExpand: (!!contentRef || canExpandInline) && fullContent === null,
    disabled: (!!contentRef && !chatId) || loading,
    label: fullResponseLabel(loading, contentBytes),
    error,
    load,
  };
}

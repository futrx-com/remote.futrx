export const API_RESPONSE_STATUS = {
  noContent: 204,
  unauthorized: 401,
  notFound: 404,
  conflict: 409,
} as const;

export const DEFAULT_CHAT_HISTORY_COMMIT_LIMIT = 100;
/** Complete conversation turns fetched on first load and on each older page. */
export const CHAT_TRANSCRIPT_TURN_PAGE_LIMIT = 20;
export const DIRTY_WORKING_TREE_FALLBACK_MESSAGE = "dirty working tree";
export const DEFAULT_UPLOAD_MEDIA_TYPE = "application/octet-stream";
export const CHAT_STREAM_MESSAGE_TYPES = {
  prompt: "prompt",
  cancel: "cancel",
	interactionResponse: "interaction_response",
} as const;
/** Usage records fetched per drill-down page. */
export const USAGE_RECORD_PAGE_LIMIT = 100;
/** How long typing settles before the workspace file search is sent. */
export const WORKSPACE_FILE_SEARCH_DEBOUNCE_MS = 250;

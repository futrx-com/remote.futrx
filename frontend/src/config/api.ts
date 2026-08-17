export const API_RESPONSE_STATUS = {
  noContent: 204,
  unauthorized: 401,
  notFound: 404,
  conflict: 409,
} as const;

export const DEFAULT_CHAT_HISTORY_COMMIT_LIMIT = 100;
export const DEFAULT_AUDIT_LOG_LIMIT = 50;
export const DIRTY_WORKING_TREE_FALLBACK_MESSAGE = "dirty working tree";
export const DEFAULT_UPLOAD_MEDIA_TYPE = "application/octet-stream";
export const CHAT_STREAM_MESSAGE_TYPES = {
  prompt: "prompt",
  cancel: "cancel",
} as const;

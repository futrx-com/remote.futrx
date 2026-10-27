/** How often the settings page re-reads server info while it is open. */
export const SERVER_INFO_REFRESH_INTERVAL_MS = 15_000;
/** How often an open administrator workspace checks for a newer release. */
export const SELF_UPDATE_CHECK_INTERVAL_MS = 60 * 60 * 1_000;
/** How often update status is re-read while an update is actually running. */
export const SELF_UPDATE_RUNNING_POLL_INTERVAL_MS = 3_000;

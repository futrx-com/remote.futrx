import { STORAGE_KEYS } from "../../config/storageKeys.ts";
import { browserStorageService } from "../../services/platform/browserStorageService.ts";

/**
 * Whether the last session on this browser got all the way into the workspace.
 *
 * The very first auth check has no idea whether it will land on a login screen
 * or on the workspace, and guessing wrong in either direction is jarring: a
 * workspace outline in front of a stranger, or a spinner in front of the
 * returning user this whole change is about. The previous outcome is the best
 * available signal, so it is remembered and used to pick which one to paint.
 */
export function expectsWorkspace(): boolean {
  return browserStorageService.readBool(STORAGE_KEYS.workspaceBoot);
}

export function rememberWorkspaceBoot(reached: boolean): void {
  browserStorageService.writeBool(STORAGE_KEYS.workspaceBoot, reached);
}

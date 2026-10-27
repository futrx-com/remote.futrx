import { sendHttpRequest } from "../transport/http.ts";
import { FRONTEND_BUILD } from "../config/build.ts";
import type { FrontendBuildManifest } from "../models/frontendBuild";

export const frontendBuildApi = {
  /** The build the server serves now, or null when it cannot say. */
  served: async (): Promise<string | null> => {
    const response = await sendHttpRequest("GET", FRONTEND_BUILD.manifestUrl, undefined, {
      cache: "no-store",
    });
    if (!response.ok) return null;
    const manifest = (await response.json()) as Partial<FrontendBuildManifest> | null;
    return typeof manifest?.build === "string" && manifest.build ? manifest.build : null;
  },
};

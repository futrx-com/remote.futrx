import { defineConfig, type Plugin } from "vite";
import preact from "@preact/preset-vite";
import { createHash } from "node:crypto";
import { resolve } from "node:path";
import { FRONTEND_BUILD } from "./src/config/build";

// Vite writes the built bundle into backend/public/ at build time.
// Go's //go:embed public (inside backend/) picks it up at the next `go build`.
export default defineConfig({
  plugins: [preact(), frontendBuildStamp()],
  build: {
    outDir: resolve(__dirname, "../backend/public"),
    emptyOutDir: true,
    sourcemap: false,
    target: "es2020",
    chunkSizeWarningLimit: 600,
  },
  server: {
    port: 5174,
    proxy: {
      // Dev: Vite serves the SPA, Go (on :7682 locally) handles API + WS.
      "/api": "http://127.0.0.1:7682",
      "/ws": { target: "ws://127.0.0.1:7682", ws: true },
    },
  },
});

// Stamps the build so an open page can tell it has been superseded (see
// src/config/build.ts). The stamp hashes the finished index.html, whose script
// and stylesheet names carry content hashes of everything they load; it is
// then written into that page as a meta tag and beside it as the manifest.
function frontendBuildStamp(): Plugin {
  return {
    name: "remote-frontend-build-stamp",
    apply: "build",
    enforce: "post",
    generateBundle(_options, bundle) {
      const page = bundle["index.html"];
      if (page?.type !== "asset") {
        this.error("index.html is missing from the bundle, so the build cannot be stamped");
      }
      const html = String(page.source);
      const build = createHash("sha256").update(html).digest("hex").slice(0, 16);
      page.source = html.replace(
        "</head>",
        `  <meta name="${FRONTEND_BUILD.metaName}" content="${build}" />\n  </head>`
      );
      this.emitFile({
        type: "asset",
        fileName: FRONTEND_BUILD.manifestFile,
        source: `${JSON.stringify({ build })}\n`,
      });
    },
  };
}

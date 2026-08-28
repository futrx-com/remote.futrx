# vendors/ — pinned fallback copies of fragile upstream downloads

Some upstreams this project installs from are not reliably reachable from
every server. The concrete case that created this folder: **Google's
Chrome-for-Testing CDN geo-blocks some datacenter IP ranges** (`403 —
"We're sorry, but this service is not available in your location"`, observed
on Hetzner and reported on Scaleway). On an affected server,
`npx playwright install chromium` can never succeed, which used to kill the
base-image build with a misleading `exit status 2`.

The fix is self-contained in the repo, costs nothing to operate, and needs no
configuration from self-hosters:

1. **Every pin lives in [`versions.env`](../backend/internal/agent/provisioning/versions.env)**
   (symlinked at `infra/versions.env`) — agent CLIs, Node, Go, code-server,
   and the Playwright/CfT set including sha256 pins for the vendored assets.
2. **A GitHub Action republishes the pinned archives** as release assets on
   this repo (tag `PW_VENDOR_RELEASE_TAG`, e.g. `vendors-playwright-1.60.0`).
   GitHub release bandwidth is free and its CDN is not geo-blocked from the
   affected hosts. The workflow
   ([`.github/workflows/vendor-playwright.yml`](../.github/workflows/vendor-playwright.yml))
   downloads the amd64 and arm64 archives from Playwright's CDN on a GitHub
   runner, verifies all six sha256 pins, uploads them, and round-trips the
   published URLs.
3. **The installer falls back automatically.**
   [`agent-browser-install.sh`](../backend/internal/integration/containers/browser/assets/agent-browser-install.sh)
   tries the normal direct download first; on failure it fetches the vendored
   assets for the host's Debian architecture, **verifies them against the
   sha256 pins baked in from `versions.env`**, and serves them to Playwright
   from a loopback HTTP server
   (`PLAYWRIGHT_DOWNLOAD_HOST=http://127.0.0.1:8377`), so Playwright's own
   installer logic — cache paths, architecture-specific executable layouts,
   revision directories, and completion markers — runs unmodified.
   Playwright's URL layout under a custom download host has changed across
   releases; serving by basename sidesteps that.

The binaries are byte-identical to the upstream ones (the release notes and
workflow logs are the provenance chain), so the fallback adds no third-party
trust — unlike pointing installs at public third-party mirrors.

## Bumping the Playwright pin

1. Edit `versions.env`: set `PLAYWRIGHT_VERSION`, then update
   `PW_CFT_VERSION`, `PW_CHROMIUM_BUILD`, and `PW_FFMPEG_BUILD` to what the
   publisher's amd64 and arm64 Playwright dry-runs report, and
   `PW_VENDOR_RELEASE_TAG` (new tag — existing tags are never moved; the
   updater's `git fetch --tags` cannot tolerate moved tags).
2. Run `bash vendors/publish-playwright-assets.sh` from a machine with clean
   egress — on a sha mismatch it prints the new hashes to paste into
   `versions.env`. Or merge the pin bump and let the workflow run; it fails
   with the same instructions if the sha pins are stale.
3. Merged to `main`, the workflow re-verifies and publishes the new release.

## If both download paths fail on your server

Your server's IP is likely geo-blocked by Google *and* unable to reach GitHub
releases (or the release for the pinned version has not been published on
your fork — run the workflow once). Escape hatches:

- `HTTPS_PROXY` is honored natively by Playwright's downloader if you have a
  proxy with clean egress.
- Hetzner asks affected customers to report the IP + blocked endpoint via
  [console.hetzner.cloud/support](https://console.hetzner.cloud/support);
  they escalate misclassified ranges to Google.
- Forks: set `PW_VENDOR_REPO` in `versions.env` to your fork's slug so the
  fallback pulls from your own releases.

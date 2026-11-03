# Browser download fallback

The browser pins and SHA-256 hashes live in
[`versions.env`](../backend/internal/agent/provisioning/versions.env).
Chrome for Testing is pinned **independently** of Playwright's bundled Chromium,
so browser security fixes do not have to wait for a Playwright release.
Both Linux x64 and ARM64 use the exact same Chrome version.

The installer downloads Chrome and FFmpeg from their official upstreams and
checks the committed SHA-256 hashes before extraction. When an upstream is
unreachable or geo-blocks a server, it uses the identical archives from the
`PW_VENDOR_RELEASE_TAG` release in `PW_VENDOR_REPO`. A checksum mismatch fails
the installation; it never selects an older browser from a project cache.

The managed runtime lives in `/opt/remote-browser/pw-<driver>-chrome-<browser>`.
It includes the pinned Playwright driver, full Chrome (used for both headed
and headless browsing), and the driver's FFmpeg build for video recording.
Workspace Playwright installations do not select Remote's browser executable.
The existing live profile remains in `/workspace/.browser-gui/profile`.
An existing browser core with a different version is restarted on next start.

## Updating the browser

1. Select a patched stable version from Google's
   [Chrome for Testing metadata](https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json).
   Update `PW_CFT_VERSION`, both Chrome archive hashes, and use a new
   `PW_VENDOR_RELEASE_TAG`. Existing release tags must never move.
2. When updating `PLAYWRIGHT_VERSION`, also check the FFmpeg build with
   `npx playwright@<version> install ffmpeg --dry-run` and update both FFmpeg
   archive hashes. Chrome does not need to match the driver's browser manifest.
3. Run `bash vendors/publish-playwright-assets.sh` from clean egress, or run
   the **Vendor Playwright assets** workflow against the candidate branch.
   The publisher verifies all four archives and their published downloads.
4. Test fresh browser provisioning and `infra/qa/update.sh <ref>` against an
   existing QA installation. Exercise both the live Browser pane and headless
   screenshots/recording. Browser/driver compatibility needs a real smoke test.

The main-branch workflow publishes assets automatically after pin changes.
For forks, change `PW_VENDOR_REPO` to the fork's repository and publish there.
The fallback release must exist before a geo-blocked server can install the
new runtime.

This update does not enable Chromium's sandbox or isolate the browser from
project credentials. Those require a dedicated user and LXC/AppArmor testing.

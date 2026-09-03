# design-sync notes — remote.futrx

- This repo is an APP (Preact + Vite + Tailwind), not a packaged design system. The sync scope is a curated set of presentational components, exported via `frontend/ds-entry.ts` (the `--entry` for the converter). Components wired to `src/state`/`src/transport` are deliberately out of scope — check imports before adding one.
- **Preact→React bridge**: the app is Preact, Claude Design renders React. `.design-sync/tsconfig.sync.json` `paths` map `preact`, `preact/hooks`, `preact/jsx-runtime`, `preact/compat` to `.design-sync/preact-shim/*.ts`, which re-export React equivalents; the converter's reactShim then externalizes React to `window.React`. The app's UI only uses `createContext` + standard hooks, so the shims are complete. If a component starts using another Preact API (`toChildArray`, `render`, class components with preact-specific lifecycles), extend the shims.
- JSX compiles to classic `React.createElement` (free `React` global — resolved by `_vendor/react.js` at preview time). Components use `class=` attributes; React renders them (with console warnings) and Tailwind applies fine. Kebab-case SVG attrs also pass through.
- **No library build exists.** `.d.ts` extraction needs a declaration tree: `buildCmd` runs `tsc ds-entry.ts --declaration --emitDeclarationOnly --declarationDir .ds-dts` and `frontend/package.json#types` points at `.ds-dts/ds-entry.d.ts`. Without this the emitted props are `[key: string]: unknown` stubs. `.ds-dts/` is gitignored — buildCmd regenerates it.
- **react/react-dom/@types/react are NOT app deps** — buildCmd installs them `--no-save` into `frontend/node_modules` (react@18 for the UMD vendor build). A fresh `npm ci` wipes them; always run buildCmd before the converter.
- **CSS is generated, not shipped**: `buildCmd` compiles Tailwind via `.design-sync/tailwind.sync.ts` (app theme + content over `frontend/src` AND `.design-sync/previews/` + safelist of `(bg|text|border|ring|divide)-(ink|accent)-*` with hover/focus/group-hover/disabled variants) to `frontend/.ds-css/tailwind.css` (`cfg.cssEntry`). Utility classes not in app source and not safelisted DO NOT EXIST in the shipped sheet — preview authors and the design agent must stick to that vocabulary or inline styles.
- **Dark ground is mandatory**: the app renders on `#0f1014` (ink-900). The preview-card harness hard-codes a white body, so `cfg.provider = DsSurface` (a sync-only component in `frontend/ds-surface.tsx`, exported from ds-entry, excluded from cards via `componentSrcMap: null`) wraps every preview in the dark surface. Without it, most components are invisible on white.
- Icons (`src/ui/primitives/icons.tsx`, ~60 exports) ship via `cfg.extraEntries` — importable from the bundle but not component cards.
- Component discovery relies on `componentSrcMap` pins (the .d.ts tree has no index; `exportedNames` works off `.ds-dts`). To add a component: export it in `ds-entry.ts` AND pin it in `componentSrcMap`.
- Playwright: cached chromium build 1234 pins playwright@1.62.x (installed in `.ds-sync/`). `browsers.json` must pin revision 1234.

## Preview-authoring recipes (learned wave 1)
- **Class availability rule**: only Tailwind classes occurring in `frontend/src` plus the safelist (`(bg|text|border|ring|divide)-(ink|accent)-*`, `font-sans`, `max-w-xs…4xl`) exist in the shipped sheet. Verify with `grep <class> ds-bundle/_ds_bundle.css`; otherwise inline `style={}`. Preview-only classes DO land in the sheet, but only after a full buildCmd + package-build (preview-rebuild does not regenerate CSS).
- **Fixed-position components** (LoadingScreen renders `.app-shell`, `position:fixed` + `height:var(--app-height,100vh)`): contain with a wrapper carrying `transform: translateZ(0)` (makes it the fixed containing block), a fixed height, and `"--app-height": "100%"`.
- **Self-positioned overlays** (ComposerDropOverlay is `absolute inset-x-3 -top-16`): sized outer container with `paddingTop` ≥ the escape distance — `marginTop` on the inner anchor collapses through and the overlay escapes the card.
- **Absolute-positioned buttons** (JumpToLatestButton `absolute right-4 bottom-4`): plain `relative` fixed-height stage suffices.
- **StreamingText with `streaming={true}`**: typewriter from empty at ~80cps; keep text under ~100 chars so capture lands mid-reveal with visible content.
- **AskUserQuestion**: reads localStorage `askq-answered:<toolUseId>` — use unique toolUseIds per export. Input shape `{questions:[{question, header?, multiSelect?, options:[{label, description?}]}]}`. QuestionProgress returns null for total ≤ 1.
- **DiffView**: needs real `diff --git` + `---/+++` + `@@` hunks; `/dev/null` drives NEW/DELETED badges; `Binary files … differ` drives BINARY; unparseable → raw pre.
- **Bare-body components** (CodeBlock is an unstyled `<pre>`): frame in the app's card idiom (`rounded-lg border border-white/10 bg-white/[0.04]`).
- **AttachmentChip states are derived**: pending = no `serverPath` and no `error`; image branch needs `isImage` AND `objectUrl` (inline SVG data URI works).
- Tiny text (9px badges) can look blank on downscaled sheets — zoom-crop the native PNG before failing a cell.
- **Popover components** (ModelPicker `open` is a controlled prop): flex wrappers need `items-start` (stretch detaches `absolute top-full` menus) and an explicit `minHeight` to contain the open menu. `modelRef` accepts `{current: null}`.
- **Full-bleed panes** (BrowserEmptyState, NoChatSelected): frame with `border border-white/10 rounded-lg overflow-hidden` + explicit height so the pane edge reads intentionally.
- Realistic data sources: capability shapes in `frontend/src/models/agentCapabilities.ts` and provider fixtures under `backend/internal/agent/*/capabilities_test.go`; default cwd `/opt/remote.futrx` in `frontend/src/ui/chat/ideLinks.ts`. Runtime model catalogs come from the backend API, not a compiled frontend catalog.

## App-level findings (for the maintainers, not sync issues)
- WorkspaceActions expanded state: `text-accent-blue`/`border-accent-blue/40` are dead — the base `text-ink-200`/`border-white/10` utilities come later in the generated Tailwind CSS and win, so an expanded button only gets the brighter background (`frontend/src/ui/chat/header/WorkspaceActions.tsx:193`).

## Known render warns
- SendControls: `Disconnected` and `DisabledEmpty` are visually identical by design (difference is the `title` tooltip) — a `variants render identically` warn here is expected.
- Hover/focus-only affordances are absent from captures by design (ChatRow row actions, ProjectGroup hover bg, WorkspaceSearch focus ring, QuestionOption hover, JumpToLatestButton hover).
- ProjectStatusDot provisioning uses `animate-pulse` — captured opacity varies between runs.

## Re-sync risks
- `react@18 --no-save` installs vanish on `npm ci` — buildCmd re-installs them; don't skip it.
- The Tailwind sheet is content-scanned: preview edits can introduce classes that only exist after the next full buildCmd + converter run. Subagent-scoped `preview-rebuild` does NOT regenerate CSS — a full rebuild must land before upload.
- `frontend/package.json#types` points at a gitignored path (`.ds-dts/`); harmless for the app, but if the app ever publishes, revisit.
- The Preact→React shims silently under-cover new Preact APIs — a preview that throws `X is not a function` from the bundle usually means a missing shim export.

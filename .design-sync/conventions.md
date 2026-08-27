# remote.futrx UI — build conventions

**Dark-only system.** Every screen sits on the ink-900 ground (`#0f1014`). Wrap each app root in `DsSurface` (exported from this bundle) — it applies `bg-ink-900 text-ink-100 font-sans antialiased p-6`. Without it, components render near-invisible on a white page (their borders are `border-white/10`-style translucents that assume the dark ground). If you build your own root instead, give it `bg-ink-900 text-ink-100 min-h-screen`.

**Styling idiom: Tailwind utility classes — but only classes present in `styles.css` exist.** The stylesheet is compiled; a utility that is not in it silently does nothing. Safe vocabulary:

- Neutrals: `ink-50 … ink-900` (light→dark). Text hierarchy the app uses: `text-ink-100` primary, `text-ink-300` secondary, `text-ink-400` muted. Surfaces: `bg-ink-800`/`bg-ink-900`; hairlines: `border-white/10`, subtle fills `bg-white/5`.
- Accents: `accent-blue #8ab4ff`, `accent-green #7bd88f`, `accent-red #ff7b72`, `accent-yellow #e2b86d`, `accent-orange #f0a45d`, `accent-purple #b8a8ff` — as `text-accent-*`, `bg-accent-*`, `border-accent-*`, `ring-accent-*` (full families are safelisted, incl. `hover:`/`focus:`/`group-hover:`/`disabled:`). Tinted fills follow the app's pattern: `bg-accent-blue/15` + `border-accent-blue/30`.
- Type: `font-sans` (system stack) for UI, `font-mono` for code/paths; sizes commonly `text-sm`, `text-[12.5px]`, `text-[14.5px]`.
- Standard layout/spacing/rounding utilities used across the app (`flex`, `gap-2`, `p-3`, `rounded-lg`, `truncate`, …) are all present. For an exotic one-off, prefer an inline `style={}` over inventing a class.

**App-shell classes** (from the app's own component layer, usable as-is): `codex-window-frame`, `codex-sidebar`, `codex-main`, `codex-header`, `codex-thread`, `codex-user-bubble`, `codex-tool-shell`, `codex-composer-card`, `codex-icon-button`, `codex-send-button`, `md-code` (+ `md-code-keyword`/`-string`/`-comment`/… for syntax tones), `touch-scroll`, `no-scrollbar`.

**Icons ship in the bundle** as ~1em stroke SVG components taking a `class` prop: `Terminal`, `Loader` (pair with `animate-spin`), `ChevronDown`, `ChevronRight`, `AlertCircle`, `Activity`, `X`, `File`, `RotateCcw`, `Search`, `Plus`, `Settings`, and more — check the bundle's export list before inventing an SVG.

**Where the truth lives**: `styles.css` → `_ds_bundle.css` (the full compiled sheet — read it to confirm a class exists); each component's `.d.ts` for props and `.prompt.md` for usage.

**Idiomatic composition**:

```tsx
import { DsSurface, ToolShell, CodeBlock, UserMessage, Terminal } from "remote.futrx-web";

<DsSurface>
  <div className="max-w-xl flex flex-col gap-2">
    <UserMessage text="Run the build" t={1} />
    <ToolShell icon={<Terminal class="w-4 h-4" />} label="npm run build"
               badge="exit 0" status="done" defaultOpen>
      <CodeBlock text={"vite v8.0.5 building...\n✓ built in 3.42s"} />
    </ToolShell>
  </div>
</DsSurface>
```

Note: these components accept `class` (Preact heritage) — when composing THEM pass `class` to icons as shown; for your own JSX elements use `className` as usual (both resolve).

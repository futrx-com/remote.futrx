# remote.futrx UI — build conventions

**Token-driven, dual theme.** Colour comes from CSS custom properties declared once per theme in `frontend/src/index.css` (`:root`/`html[data-theme="dark"]` and `html[data-theme="light"]`). No component hard-codes a hex value, and there is no `!important` theme-override layer any more — a utility that resolves through a token is automatically correct in both themes. Wrap each app root in `DsSurface` (exported from this bundle); it applies the app ground and base text colour. If you build your own root instead, give it `bg-canvas text-ink-100 min-h-screen`.

**Styling idiom: Tailwind utility classes — but only classes present in `styles.css` exist.** The stylesheet is compiled; a utility that is not in it silently does nothing. Safe vocabulary:

- **Elevation** (deepest → highest): `bg-app` (frame behind the window cards), `bg-canvas` (thread/main), `bg-surface` (sidebar, headers, cards, composer), `bg-raised` (menus, popovers), `bg-inset` (inputs, code bodies, wells).
- **Hairlines and fills**: `border-line` and `border-line-strong` for rules; `bg-tint`, `bg-tint-strong`, `bg-tint-active` for translucent surfaces (rest → hover → selected). These flip polarity per theme, so never reach for `bg-white/5` or `border-white/10` — they are gone from the codebase.
- **Text**: `text-ink-50` brightest, `text-ink-100` primary, `text-ink-200` secondary, `text-ink-300` muted, `text-ink-400` subtle, `text-ink-500` faint (non-text). Sizes commonly `text-[13px]`, `text-[12.5px]`, `text-[14.5px]`; `font-sans` for UI, `font-mono` for code and paths.
- **Accents**: `accent-blue`, `accent-green`, `accent-red`, `accent-yellow`, `accent-orange`, `accent-purple` — as `text-accent-*`, `bg-accent-*`, `border-accent-*`, `ring-accent-*`, with `/opacity` modifiers (`bg-accent-blue/[0.14]`). On a *filled* accent, use `text-on-accent` — never `text-white`, which is unreadable on the light-blue dark-theme accent.
- **Buttons**: `btn` plus one intent — `btn-primary`, `btn-secondary`, `btn-ghost`, `btn-danger` — and optionally `btn-sm` / `btn-lg` / `btn-block`. A segmented control is `segmented` + `segmented-option` (selected via `aria-checked="true"`).
- **Rounding**: `rounded-control` (0.5rem) for controls, `rounded-card` (0.75rem) for cards and panels, `rounded-panel` (1rem) for the composer and large surfaces. **Elevation**: `shadow-pop` for menus, `shadow-modal` for dialogs.
- Standard layout/spacing utilities (`flex`, `gap-2`, `p-3`, `truncate`, …) are all present. For an exotic one-off, prefer an inline `style={}` over inventing a class.

**App-shell classes** (from the app's own component layer, usable as-is): `codex-window-frame`, `codex-sidebar`, `codex-main`, `codex-header`, `codex-thread`, `codex-user-bubble`, `codex-tool-shell`, `codex-composer-card`, `codex-icon-button`, `codex-send-button`, `codex-prose`, `md-code` (+ `md-code-keyword`/`-string`/`-comment`/… for syntax tones), `touch-scroll`, `no-scrollbar`.

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

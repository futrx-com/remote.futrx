// Design-sync Tailwind build: same theme as the app, but content also covers
// the authored preview files, and the ink/accent color families are safelisted
// so the claude.ai/design agent can use the full palette vocabulary even where
// the app itself doesn't. Run from frontend/:
//   npx tailwindcss -c ../.design-sync/tailwind.sync.ts -i src/index.css -o .ds-css/tailwind.css
import base from "../frontend/tailwind.config";

export default {
  ...base,
  content: [
    "./index.html",
    "./src/**/*.{ts,tsx}",
    "./ds-entry.ts",
    "../.design-sync/previews/**/*.tsx",
  ],
  safelist: [
    "font-sans",
    { pattern: /^max-w-(xs|sm|md|lg|xl|2xl|3xl|4xl)$/ },
    {
      pattern: /^(bg|text|border|ring|divide)-(ink|accent)-.+$/,
      variants: ["hover", "focus", "group-hover", "disabled"],
    },
    // Semantic surface/line/tint tokens: the app's real vocabulary since the
    // theme moved to CSS custom properties.
    {
      pattern: /^bg-(app|canvas|surface|raised|inset|tint|tint-strong|tint-active)$/,
      variants: ["hover", "focus", "group-hover", "disabled"],
    },
    {
      pattern: /^(border|divide|ring)-(line|line-strong)$/,
      variants: ["hover", "focus", "group-hover"],
    },
    "text-on-accent",
    { pattern: /^rounded-(control|card|panel)$/ },
    { pattern: /^shadow-(pop|modal)$/ },
  ],
};

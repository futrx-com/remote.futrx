import type { Config } from "tailwindcss";

// Every colour resolves through a CSS custom property so a single token block
// in index.css drives both themes. Channel triplets (`--fg: 227 227 231`) keep
// Tailwind's `/opacity` modifiers working on the semantic scales.
const channel = (token: string) => `rgb(var(${token}) / <alpha-value>)`;

export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        // Text scale. Kept under the historical `ink` name so existing markup
        // keeps working, but the steps now mean something: 50-200 are content,
        // 300-400 are secondary, 500+ are non-text (disabled fills, rules).
        ink: {
          50:  channel("--fg-bright"),
          100: channel("--fg"),
          200: channel("--fg-dim"),
          300: channel("--fg-muted"),
          400: channel("--fg-subtle"),
          500: channel("--fg-faint"),
          600: channel("--bg-raised-rgb"),
          700: channel("--bg-surface-rgb"),
          800: channel("--bg-canvas-rgb"),
          900: channel("--bg-app-rgb"),
        },
        // Elevation scale, deepest to highest.
        app: channel("--bg-app-rgb"),
        canvas: channel("--bg-canvas-rgb"),
        surface: channel("--bg-surface-rgb"),
        raised: channel("--bg-raised-rgb"),
        inset: channel("--bg-inset-rgb"),
        // Legible foreground on top of a filled accent, in either theme.
        "on-accent": channel("--on-accent"),
        // Hairlines and translucent fills. These flip polarity per theme, so
        // they carry their own alpha rather than taking a modifier.
        line: {
          DEFAULT: "var(--line)",
          strong: "var(--line-strong)",
        },
        tint: {
          DEFAULT: "var(--tint)",
          strong: "var(--tint-strong)",
          active: "var(--tint-active)",
        },
        accent: {
          blue:   channel("--accent-blue"),
          green:  channel("--accent-green"),
          red:    channel("--accent-red"),
          yellow: channel("--accent-yellow"),
          orange: channel("--accent-orange"),
          purple: channel("--accent-purple"),
        },
      },
      fontFamily: {
        mono: ['ui-monospace', '"SF Mono"', 'Menlo', 'Consolas', 'monospace'],
        sans: ['-apple-system', 'system-ui', '"Segoe UI"', 'sans-serif'],
      },
      borderRadius: {
        // One rounding rhythm: control -> card -> panel.
        control: "0.5rem",
        card: "0.75rem",
        panel: "1rem",
      },
      boxShadow: {
        // Elevation is carried by two shadows only, so popovers and modals
        // across the app read as the same material.
        pop: "var(--shadow-pop)",
        modal: "var(--shadow-modal)",
      },
      animation: {
        "pulse-fast": "pulse 1s cubic-bezier(0.4, 0, 0.6, 1) infinite",
      },
    },
  },
  plugins: [],
} satisfies Config;

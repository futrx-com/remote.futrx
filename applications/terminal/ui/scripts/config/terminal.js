export const THEME = {
  // Remote's dark pane surface (--bg-surface-rgb).
  background: "#141519",
  foreground: "#e4e4e7",
  cursor: "#f4f4f5",
  selectionBackground: "#3f4047",
  black: "#18191e",
  red: "#ff7b72",
  green: "#7bd88f",
  yellow: "#e2b86d",
  blue: "#8ab4ff",
  magenta: "#b8a8ff",
  cyan: "#7dd3fc",
  white: "#e4e4e7",
  brightBlack: "#707078",
  brightRed: "#ff9b96",
  brightGreen: "#b7f7c2",
  brightYellow: "#f0d28a",
  brightBlue: "#a7c7ff",
  brightMagenta: "#c8bbff",
  brightCyan: "#a5f3fc",
  brightWhite: "#f4f4f5",
  // The ruler shares the scrollbar's column. Its border is painted on a canvas
  // in this colour and defaults to white, so it is given the background's.
  overviewRulerBorder: "#141519",
  scrollbarSliderBackground: "rgba(228, 228, 231, 0.18)",
  scrollbarSliderHoverBackground: "rgba(228, 228, 231, 0.32)",
  scrollbarSliderActiveBackground: "rgba(228, 228, 231, 0.44)",
};

// Find highlights every match through the search addon's decorations, which
// xterm still ships as proposed API.
export const SEARCH_OPTIONS = {
  decorations: {
    matchBackground: "#4a3f1f",
    matchOverviewRuler: "#e2b86d",
    activeMatchBackground: "#8a6d1f",
    activeMatchColorOverviewRuler: "#f0d28a",
  },
};

import type { ITerminalOptions, ITheme } from "@xterm/xterm";

export const TERMINAL_THEME: ITheme = {
  background: "#0f1014",
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
};

export const TERMINAL_OPTIONS: ITerminalOptions = {
  cursorBlink: true,
  convertEol: true,
  fontFamily: "ui-monospace, SFMono-Regular, SF Mono, Menlo, Consolas, monospace",
  fontSize: 13,
  lineHeight: 1.18,
  scrollback: 6_000,
};

export const TERMINAL_WEB_SOCKET_BINARY_TYPE = "arraybuffer";
export const TERMINAL_CONNECTION_ERROR_MESSAGE = "Terminal connection failed.";
export const TERMINAL_OVERLAY_LOAD_ERROR_MESSAGE =
  "Terminal failed to load. The app may have updated in the background — retry, or refresh the page.";
export const TERMINAL_DEFAULT_TITLE = "workspace";
export const TERMINAL_INITIAL_FIT_DELAY_MS = 0;
export const TERMINAL_STATUS = {
  connecting: "connecting",
  connected: "connected",
  closed: "closed",
  error: "error",
} as const;
export const TERMINAL_MESSAGE_TYPES = {
  input: "input",
  resize: "resize",
} as const;

import { FitAddon } from "./vendor/addon-fit.mjs";
import { SearchAddon } from "./vendor/addon-search.mjs";
import { Terminal } from "./vendor/xterm.mjs";
import { afterClose, embedderOrigin, retryDelayMs } from "./reconnect.js";

const THEME = {
  // Remote's dark pane surface (--bg-surface-rgb), which this page cannot read
  // across origins.
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
const SEARCH_OPTIONS = {
  decorations: {
    matchBackground: "#4a3f1f",
    matchOverviewRuler: "#e2b86d",
    activeMatchBackground: "#8a6d1f",
    activeMatchColorOverviewRuler: "#f0d28a",
  },
};

const query = new URLSearchParams(window.location.search);
const host = document.getElementById("terminal");
const terminal = new Terminal({
  allowProposedApi: true,
  cursorBlink: true,
  convertEol: true,
  fontFamily: "ui-monospace, SFMono-Regular, SF Mono, Menlo, Consolas, monospace",
  fontSize: 13,
  lineHeight: 1.18,
  scrollback: 6000,
  // Also the width of xterm's scrollbar, which shares the ruler's column.
  overviewRuler: { width: 8 },
  theme: THEME,
});
const fit = new FitAddon();
const search = new SearchAddon();
terminal.loadAddon(fit);
terminal.loadAddon(search);
terminal.open(host);

let socket = null;
let attempt = 0;
let retryTimer = null;
let ended = false;
let visible = false;

// The drawer's header shows the state; it lives on Remote's origin, so it is
// told by message, addressed to that origin only.
const embedder = window.parent === window ? null : embedderOrigin(window.location);
function report(status) {
  if (embedder) window.parent.postMessage({ source: "remote-terminal", status }, embedder);
}

function socketUrl() {
  const url = new URL("ws", window.location.href);
  url.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  url.search = "";
  url.searchParams.set("session", query.get("session") ?? "");
  url.searchParams.set("cwd", query.get("cwd") ?? "");
  return url.toString();
}

function send(message) {
  if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify(message));
}

function fitAndResize() {
  // A collapsed pane measures 0×0. Fitting to that would resize the shell to
  // zero columns and mangle its current line, so wait until it is shown.
  if (!visible) return;
  try {
    fit.fit();
  } catch (error) {
    console.error("[terminal] fit failed", error);
    return;
  }
  send({ type: "resize", cols: terminal.cols, rows: terminal.rows });
}

function connect() {
  window.clearTimeout(retryTimer);
  retryTimer = null;
  ended = false;
  report(attempt === 0 ? "connecting" : "reconnecting");

  const current = new WebSocket(socketUrl());
  current.binaryType = "arraybuffer";
  socket = current;
  current.onopen = () => {
    if (socket !== current) return;
    attempt = 0;
    // The service replays the session's recent output on every attach, so
    // what is on screen from the previous connection would appear twice.
    terminal.reset();
    report("connected");
    fitAndResize();
  };
  current.onmessage = (event) => {
    if (socket !== current) return;
    terminal.write(typeof event.data === "string" ? event.data : new Uint8Array(event.data));
  };
  current.onclose = (event) => {
    if (socket !== current) return;
    socket = null;
    const step = afterClose({ closeCode: event.code, visible });
    if (step === "ended") {
      ended = true;
      report("ended");
      terminal.write("\r\n\x1b[2m[Shell exited. Press Enter to start a new one.]\x1b[0m\r\n");
      return;
    }
    report("reconnecting");
    if (step === "retry") {
      retryTimer = window.setTimeout(connect, retryDelayMs(attempt));
      attempt += 1;
    }
  };
}

/** Reconnect now if the pane is showing a connection that was lost. */
function resume() {
  if (!visible || socket || ended) return;
  attempt = 0;
  connect();
}

terminal.onData((data) => {
  if (ended) {
    if (data === "\r") {
      attempt = 0;
      connect();
    }
    return;
  }
  send({ type: "input", data });
});

new ResizeObserver(() => {
  const shown = host.clientWidth > 0 && host.clientHeight > 0;
  if (shown === visible) {
    fitAndResize();
    return;
  }
  visible = shown;
  if (!visible) {
    // Stop retrying behind a closed pane; an open connection is kept.
    window.clearTimeout(retryTimer);
    retryTimer = null;
    closeFind();
    return;
  }
  fitAndResize();
  terminal.focus();
  resume();
}).observe(host);

window.addEventListener("online", resume);
window.addEventListener("focus", () => terminal.focus());

// --- Find in terminal ---

const findBar = document.getElementById("find");
const findInput = document.getElementById("find-input");
const findStatus = document.getElementById("find-status");
const findPrevious = document.getElementById("find-previous");
const findNext = document.getElementById("find-next");

function isFindShortcut(event) {
  return (event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey &&
    event.key.toLowerCase() === "f";
}

function showFindStatus(results) {
  const term = findInput.value;
  const matched = Boolean(term) && results !== null && results.resultCount > 0;
  findStatus.dataset.empty = String(Boolean(term) && !matched);
  findPrevious.disabled = !matched;
  findNext.disabled = !matched;
  if (!term) {
    findStatus.textContent = "";
  } else if (!matched) {
    findStatus.textContent = "No results";
  } else {
    // Past the highlight limit the addon still steps through matches but stops
    // saying which one is active; count from the top rather than show "0 of n".
    const position = results.resultIndex >= 0 ? results.resultIndex + 1 : 1;
    findStatus.textContent = `${position} of ${results.resultCount}`;
  }
}

function openFind() {
  findBar.hidden = false;
  findInput.focus();
  findInput.select();
}

function closeFind() {
  if (findBar.hidden) return;
  findBar.hidden = true;
  search.clearDecorations();
  terminal.clearSelection();
  showFindStatus(null);
  if (visible) terminal.focus();
}

search.onDidChangeResults(showFindStatus);

// Cmd/Ctrl+F opens find instead of reaching the shell, where Ctrl+F is ^F.
terminal.attachCustomKeyEventHandler((event) => {
  if (!isFindShortcut(event)) return true;
  event.preventDefault();
  event.stopPropagation();
  if (event.type === "keydown") openFind();
  return false;
});

findInput.addEventListener("input", () => {
  // Typing extends the current match rather than jumping past it.
  if (findInput.value) search.findNext(findInput.value, { ...SEARCH_OPTIONS, incremental: true });
  else {
    search.clearDecorations();
    showFindStatus(null);
  }
});
findInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter") {
    event.preventDefault();
    if (!findInput.value) return;
    if (event.shiftKey) search.findPrevious(findInput.value, SEARCH_OPTIONS);
    else search.findNext(findInput.value, SEARCH_OPTIONS);
  } else if (event.key === "Escape") {
    event.preventDefault();
    closeFind();
  } else if (isFindShortcut(event)) {
    event.preventDefault();
    findInput.select();
  }
});
findPrevious.addEventListener("click", () => search.findPrevious(findInput.value, SEARCH_OPTIONS));
findNext.addEventListener("click", () => search.findNext(findInput.value, SEARCH_OPTIONS));
document.getElementById("find-close").addEventListener("click", closeFind);

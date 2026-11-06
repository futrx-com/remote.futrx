import { FitAddon } from "../vendor/addon-fit.mjs";
import { SearchAddon } from "../vendor/addon-search.mjs";
import { Terminal } from "../vendor/xterm.mjs";

const THEME = {
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
const SEARCH_OPTIONS = {
  decorations: {
    matchBackground: "#4a3f1f",
    matchOverviewRuler: "#e2b86d",
    activeMatchBackground: "#8a6d1f",
    activeMatchColorOverviewRuler: "#f0d28a",
  },
};

const FIND_MARKUP = `
  <div class="find" role="search" aria-label="Find in terminal" hidden>
    <input class="find-input" type="text" placeholder="Find in terminal" aria-label="Find in terminal"
           autocomplete="off" spellcheck="false">
    <span class="find-status" aria-live="polite"></span>
    <button type="button" class="find-previous" aria-label="Previous match" title="Previous match (Shift+Enter)" disabled>
      <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 19V5M5 12l7-7 7 7"/></svg>
    </button>
    <button type="button" class="find-next" aria-label="Next match" title="Next match (Enter)" disabled>
      <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14M19 12l-7 7-7-7"/></svg>
    </button>
    <button type="button" class="find-close" aria-label="Close find" title="Close (Esc)">
      <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg>
    </button>
  </div>
  <div class="terminal-host"></div>`;

function isFindShortcut(event) {
  return (event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey &&
    event.key.toLowerCase() === "f";
}

/**
 * The terminal and its find bar inside the drawer body. It reports typing,
 * size changes and whether the pane is showing; it has no network access.
 */
export function createTerminalView(body, { onData, onResize, onVisible }) {
  const root = body.ownerDocument.createElement("div");
  root.className = "remote-terminal";
  root.innerHTML = FIND_MARKUP;
  body.append(root);
  const host = root.querySelector(".terminal-host");
  const findBar = root.querySelector(".find");
  const findInput = root.querySelector(".find-input");
  const findStatus = root.querySelector(".find-status");
  const findPrevious = root.querySelector(".find-previous");
  const findNext = root.querySelector(".find-next");

  // xterm is built only once the pane has stopped growing. Building it, and
  // fitting it every frame, would otherwise stall Remote's opening animation.
  let terminal = null;
  let fit = null;
  let search = null;
  let visible = false;
  let disposed = false;
  let settleFrame = 0;
  let resizeFrame = 0;
  let lastWidth = -1;
  let lastHeight = -1;
  let reported = "";
  // Output and notices that arrive before xterm exists, in order.
  let queued = [];
  const disposables = [];

  function fitAndResize() {
    // A collapsed pane measures 0×0. Fitting to that would resize the shell to
    // zero columns and mangle its current line, so wait until it is shown.
    if (!visible || !terminal) return;
    try {
      fit.fit();
    } catch (error) {
      console.error("[terminal] fit failed", error);
      return;
    }
    // A drag reports many sizes; the shell only hears about new ones.
    const size = `${terminal.cols}x${terminal.rows}`;
    if (size === reported) return;
    reported = size;
    onResize({ cols: terminal.cols, rows: terminal.rows });
  }

  function scheduleFit() {
    if (resizeFrame) return;
    resizeFrame = requestAnimationFrame(() => {
      resizeFrame = 0;
      fitAndResize();
    });
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
    if (findBar.hidden || !terminal) return;
    findBar.hidden = true;
    search.clearDecorations();
    terminal.clearSelection();
    showFindStatus(null);
    if (visible) terminal.focus();
  }

  function startTerminal() {
    terminal = new Terminal({
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
    fit = new FitAddon();
    search = new SearchAddon();
    terminal.loadAddon(fit);
    terminal.loadAddon(search);
    terminal.open(host);
    disposables.push(
      terminal.onData(onData),
      search.onDidChangeResults(showFindStatus),
    );
    // Cmd/Ctrl+F opens find instead of reaching the shell, where Ctrl+F is ^F.
    terminal.attachCustomKeyEventHandler((event) => {
      if (!isFindShortcut(event)) return true;
      event.preventDefault();
      event.stopPropagation();
      if (event.type === "keydown") openFind();
      return false;
    });
    for (const data of queued) terminal.write(data);
    queued = [];
    fitAndResize();
    if (visible) terminal.focus();
  }

  // The pane has settled once its size holds for a whole frame.
  function waitForSettledPane() {
    if (settleFrame || terminal || disposed) return;
    settleFrame = requestAnimationFrame(() => {
      settleFrame = 0;
      if (!visible || terminal || disposed) return;
      if (host.clientWidth === lastWidth && host.clientHeight === lastHeight) {
        startTerminal();
        return;
      }
      lastWidth = host.clientWidth;
      lastHeight = host.clientHeight;
      waitForSettledPane();
    });
  }

  findInput.addEventListener("input", () => {
    if (!search) return;
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
      if (!findInput.value || !search) return;
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
  findPrevious.addEventListener("click", () => search?.findPrevious(findInput.value, SEARCH_OPTIONS));
  findNext.addEventListener("click", () => search?.findNext(findInput.value, SEARCH_OPTIONS));
  root.querySelector(".find-close").addEventListener("click", closeFind);

  // The drawer body stays mounted while the pane is closed; a closed pane has
  // zero width.
  const observer = new ResizeObserver(() => {
    const shown = host.clientWidth > 0 && host.clientHeight > 0;
    if (shown !== visible) {
      visible = shown;
      if (!visible) {
        closeFind();
        onVisible(false);
        return;
      }
      onVisible(true);
      terminal?.focus();
    }
    if (!visible) return;
    if (terminal) scheduleFit();
    else waitForSettledPane();
  });
  observer.observe(host);

  return {
    write(data) {
      if (terminal) terminal.write(data);
      else queued.push(data);
    },
    reset() {
      if (terminal) terminal.reset();
      else queued = [];
    },
    notice(text) {
      const line = `\r\n\x1b[2m[${text}]\x1b[0m\r\n`;
      if (terminal) terminal.write(line);
      else queued.push(line);
    },
    // Before xterm exists the size is unknown; a new shell then starts at its
    // default and is resized as soon as the terminal is fitted.
    size: () => (terminal ? { cols: terminal.cols, rows: terminal.rows } : {}),
    fitAndResize() {
      // A reconnected shell may have another viewer's size; report ours again.
      reported = "";
      fitAndResize();
    },
    dispose() {
      disposed = true;
      cancelAnimationFrame(settleFrame);
      cancelAnimationFrame(resizeFrame);
      observer.disconnect();
      for (const disposable of disposables) disposable.dispose();
      terminal?.dispose();
      root.remove();
    },
  };
}

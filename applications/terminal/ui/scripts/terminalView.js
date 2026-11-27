import { FitAddon } from "../vendor/addon-fit.mjs";
import { SearchAddon } from "../vendor/addon-search.mjs";
import { Terminal } from "../vendor/xterm.mjs";
import { THEME } from "./config/terminal.js";
import { createTerminalFind } from "./terminalFind.js";

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
  // xterm is built only once the pane has stopped growing. Building it, and
  // fitting it every frame, would otherwise stall Remote's opening animation.
  let terminal = null;
  let fit = null;
  let visible = false;
  let disposed = false;
  let settleFrame = 0;
  let resizeFrame = 0;
  let lastWidth = -1;
  let lastHeight = -1;
  let reported = "";
  // Output and notices that arrive before xterm exists, in order.
  let queued = [];
  let inputSubscription = null;
  const find = createTerminalFind(root, () => visible);

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
    const search = new SearchAddon();
    terminal.loadAddon(fit);
    terminal.loadAddon(search);
    terminal.open(host);
    inputSubscription = terminal.onData(onData);
    find.bind(terminal, search);
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

  // The drawer body stays mounted while the pane is closed; a closed pane has
  // zero width.
  const observer = new ResizeObserver(() => {
    const shown = host.clientWidth > 0 && host.clientHeight > 0;
    if (shown !== visible) {
      visible = shown;
      if (!visible) {
        find.close();
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
      inputSubscription?.dispose();
      find.dispose();
      terminal?.dispose();
      root.remove();
    },
  };
}

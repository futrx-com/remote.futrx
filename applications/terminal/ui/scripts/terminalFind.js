import { SEARCH_OPTIONS } from "./config/terminal.js";

function isFindShortcut(event) {
  return (event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey &&
    event.key.toLowerCase() === "f";
}

/** Owns the find controls and their binding to a lazily created terminal. */
export function createTerminalFind(root, isVisible) {
  const findBar = root.querySelector(".find");
  const findInput = root.querySelector(".find-input");
  const findStatus = root.querySelector(".find-status");
  const findPrevious = root.querySelector(".find-previous");
  const findNext = root.querySelector(".find-next");
  let terminal = null;
  let search = null;
  let resultsSubscription = null;

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
    if (isVisible()) terminal.focus();
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

  return {
    bind(currentTerminal, currentSearch) {
      terminal = currentTerminal;
      search = currentSearch;
      resultsSubscription = search.onDidChangeResults(showFindStatus);
      // Cmd/Ctrl+F opens find instead of reaching the shell as ^F.
      terminal.attachCustomKeyEventHandler((event) => {
        if (!isFindShortcut(event)) return true;
        event.preventDefault();
        event.stopPropagation();
        if (event.type === "keydown") openFind();
        return false;
      });
    },
    close: closeFind,
    dispose() {
      resultsSubscription?.dispose();
    },
  };
}

// The Code Server editor runs on its own origin (code--<slug>.<host>), one per
// project. Opening it with an unnamed target gives every click a fresh tab,
// so a handful of clicks on the IDE icon leaves a handful of copies of the
// same editor.
//
// The window therefore carries a stable name: the browser reuses the window it
// already knows under that name and focuses it instead of opening another
// one. The "noopener" window feature cannot be used for that, because it makes
// a browser treat a named target as "_blank" -- exactly the behaviour this
// removes. The editor runs on a different origin, so its opener reference is
// dropped by hand right after the window was opened.

const openedWindows = new Map();

// One window per editor origin. The project slug keeps the address stable
// across reinstalls, so the name survives an application upgrade.
export function ideWindowName(url) {
  return `futrx-ide-${new URL(url).host}`;
}

export function openIdeWindow(url) {
  const name = ideWindowName(url);
  const known = openedWindows.get(name);
  if (known && !known.closed) {
    known.focus();
    return known;
  }
  const opened = window.open(url, name);
  if (!opened) return null;
  opened.opener = null;
  openedWindows.set(name, opened);
  return opened;
}

import { category, formatBytes, openTitle, parentDir } from "./fileKinds.js";

const ICONS = {
  archive: '<path d="M5 4h14v16H5zM8 8h8M8 12h8M8 16h5"/>',
  audio: '<path d="M9 18V6l9-2v12M9 10l9-2"/><circle cx="6" cy="18" r="3"/><circle cx="15" cy="16" r="3"/>',
  chevron: '<path d="m9 18 6-6-6-6"/>',
  code: '<path d="m8 9-3 3 3 3M16 9l3 3-3 3M14 5l-4 14"/>',
  data: '<path d="M5 4h14v16H5zM8 8h8M8 12h8M8 16h5"/>',
  download: '<path d="M12 3v12m0 0 4-4m-4 4-4-4M5 20h14"/>',
  file: '<path d="M6 3h8l4 4v14H6zM14 3v5h5"/>',
  folder: '<path d="M3.5 7.5h6l2-2h9v13h-17z"/>',
  image: '<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="9" cy="10" r="2"/><path d="m4 17 4-4 3 3 3-3 6 6"/>',
  pdf: '<path d="M6 3h8l4 4v14H6zM14 3v5h5M8 15h8"/>',
  text: '<path d="M6 3h8l4 4v14H6zM14 3v5h5M9 12h6M9 16h6"/>',
  video: '<rect x="3" y="5" width="15" height="14" rx="2"/><path d="m18 10 3-2v8l-3-2z"/>',
};

function icon(name, className = "") {
  const span = document.createElement("span");
  span.className = `fm-icon ${className}`;
  span.setAttribute("aria-hidden", "true");
  span.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">${ICONS[name] ?? ICONS.file}</svg>`;
  return span;
}

function notice(kind, message) {
  const element = document.createElement("div");
  element.className = `fm-notice fm-notice--${kind}`;
  element.textContent = message;
  return element;
}

function empty(message) {
  const element = document.createElement("p");
  element.className = "fm-empty";
  element.textContent = message;
  return element;
}

export function renderBrowse(container, state, actions) {
  container.replaceChildren();
  const rootError = state.errorByDir.get("");
  if (rootError) container.append(notice("error", rootError));
  if (state.truncatedDirs.size > 0) {
    container.append(notice(
      "warning",
      "A folder was large and its listing was truncated. Use search or download the folder as a ZIP to get everything.",
    ));
  }
  const rootEntries = state.childrenByDir.get("") ?? [];
  if (state.rootLoading && rootEntries.length === 0) {
    container.append(empty("Loading workspace…"));
    return;
  }
  if (!state.rootLoading && !rootError && rootEntries.length === 0) {
    container.append(empty("This workspace is empty."));
    return;
  }
  container.append(renderNodes(rootEntries, 0, state, actions));
}

export function renderSearch(container, state, actions) {
  container.replaceChildren();
  if (state.searchError) container.append(notice("error", state.searchError));
  if (state.searchTruncated) {
    container.append(notice("warning", "Showing the first matches only — refine your search to narrow it down."));
  }
  const results = state.searchResults ?? [];
  if (!state.searching && !state.searchError && results.length === 0) {
    container.append(empty("No matches."));
    return;
  }
  const list = document.createElement("ul");
  list.className = "fm-tree";
  for (const node of results) list.append(renderSearchRow(node, actions));
  container.append(list);
}

function renderNodes(nodes, depth, state, actions) {
  const list = document.createElement("ul");
  list.className = depth > 0 ? "fm-tree fm-tree--nested" : "fm-tree";
  for (const node of nodes) {
    list.append(node.isDir
      ? renderFolder(node, depth, state, actions)
      : renderFile(node, actions));
  }
  return list;
}

function renderFolder(node, depth, state, actions) {
  const item = document.createElement("li");
  const row = document.createElement("div");
  row.className = "fm-row";
  const expanded = state.expanded.has(node.path);
  const loading = state.loading.has(node.path);
  const children = state.childrenByDir.get(node.path);
  const error = state.errorByDir.get(node.path);

  const main = document.createElement("button");
  main.type = "button";
  main.className = "fm-row__main";
  main.setAttribute("aria-expanded", String(expanded));
  main.addEventListener("click", () => actions.toggle(node.path));
  const chevron = icon("chevron", loading ? "fm-icon--spin" : "fm-icon--chevron");
  if (expanded && !loading) chevron.classList.add("fm-icon--expanded");
  const name = document.createElement("span");
  name.className = "fm-row__name";
  name.textContent = node.name;
  main.append(chevron, icon("folder", "fm-icon--folder"), name);
  if (children) {
    const count = document.createElement("span");
    count.className = "fm-row__meta";
    count.textContent = String(children.length);
    main.append(count);
  }
  row.append(main, downloadLink(node, actions));
  item.append(row);

  if (expanded && error) item.append(notice("inline-error", error));
  if (expanded && children?.length === 0 && !error) item.append(empty("Empty folder."));
  if (expanded && children?.length > 0) item.append(renderNodes(children, depth + 1, state, actions));
  return item;
}

function renderFile(node, actions) {
  const item = document.createElement("li");
  const row = document.createElement("div");
  row.className = "fm-row";
  const main = document.createElement("button");
  main.type = "button";
  main.className = "fm-row__main";
  main.title = openTitle(node.name);
  main.addEventListener("click", () => actions.open(node));
  const spacer = document.createElement("span");
  spacer.className = "fm-row__spacer";
  spacer.setAttribute("aria-hidden", "true");
  main.append(spacer, fileIcon(node));
  const name = document.createElement("span");
  name.className = "fm-row__name";
  name.textContent = node.name;
  main.append(name);
  if (node.size !== undefined && node.size !== null) {
    const size = document.createElement("span");
    size.className = "fm-row__meta";
    size.textContent = formatBytes(node.size);
    main.append(size);
  }
  row.append(main, downloadLink(node, actions));
  item.append(row);
  return item;
}

function renderSearchRow(node, actions) {
  const item = document.createElement("li");
  const row = document.createElement("div");
  row.className = "fm-row";
  const main = document.createElement(node.isDir ? "div" : "button");
  if (!node.isDir) {
    main.type = "button";
    main.title = openTitle(node.name);
    main.addEventListener("click", () => actions.open(node));
  }
  main.className = "fm-row__main";
  main.append(node.isDir ? icon("folder", "fm-icon--folder") : fileIcon(node));
  const labels = document.createElement("span");
  labels.className = "fm-row__labels";
  const name = document.createElement("span");
  name.className = "fm-row__name";
  name.textContent = node.name;
  labels.append(name);
  const directory = parentDir(node.path);
  if (directory) {
    const path = document.createElement("span");
    path.className = "fm-row__path";
    path.textContent = `${directory}/`;
    labels.append(path);
  }
  main.append(labels);
  if (!node.isDir && node.size !== undefined && node.size !== null) {
    const size = document.createElement("span");
    size.className = "fm-row__meta";
    size.textContent = formatBytes(node.size);
    main.append(size);
  }
  row.append(main, downloadLink(node, actions));
  item.append(row);
  return item;
}

function fileIcon(node) {
  const fileCategory = category(node.name);
  const iconName = fileCategory === "code" ? "code" : fileCategory;
  return icon(iconName, `fm-icon--${fileCategory}`);
}

function downloadLink(node, actions) {
  const link = document.createElement("a");
  link.className = "fm-row__download";
  link.href = actions.downloadUrl(node);
  if (!node.isDir) link.download = node.name;
  const title = node.isDir ? `Download ${node.name} as ZIP` : `Download ${node.name}`;
  link.title = title;
  link.setAttribute("aria-label", title);
  link.append(icon("download"));
  return link;
}

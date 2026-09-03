import { createServer } from "node:http";
import { createHash } from "node:crypto";
import { promises as fs, watch } from "node:fs";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";

const projectDirectory = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const docsDirectory = path.resolve(projectDirectory, "../docs");
const docsAssetsDirectory = path.join(docsDirectory, "assets");
const staticDirectory = path.join(projectDirectory, "static");
const outputDirectory = path.join(projectDirectory, "dist");
const homeDocumentPath = "01-overview/README.md";
const homeDocumentDirectory = path.posix.dirname(homeDocumentPath);
const privateRepositoryUrl = "https://github.com/futrx-com/remote.futrx.com";
const excludedDocumentDirectories = new Set(["assets", "codex-analysis", "fable-analysis"]);

const listFiles = async (directory, relativeDirectory = "") => {
  const entries = await fs.readdir(directory, { withFileTypes: true });
  const files = [];

  for (const entry of entries.sort((left, right) => left.name.localeCompare(right.name))) {
    const relativePath = path.posix.join(relativeDirectory, entry.name);
    const absolutePath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...await listFiles(absolutePath, relativePath));
    } else if (entry.isFile()) {
      files.push(relativePath);
    }
  }

  return files;
};

const writeStaticAssets = async () => {
  const files = await listFiles(staticDirectory);
  const hash = createHash("sha256");

  for (const relativePath of files) {
    hash.update(relativePath);
    hash.update(await fs.readFile(path.join(staticDirectory, relativePath)));
  }

  const version = hash.digest("hex").slice(0, 12);
  const assetsDirectory = path.join(outputDirectory, "assets");
  const stylesheetFilename = `site.${version}.css`;
  const scriptFilename = `site.${version}.js`;

  await fs.cp(staticDirectory, assetsDirectory, { recursive: true });
  await fs.copyFile(
    path.join(staticDirectory, "site.css"),
    path.join(assetsDirectory, stylesheetFilename),
  );

  const script = await fs.readFile(path.join(staticDirectory, "site.js"), "utf8");
  const versionedScript = script.replace(
    /(from\s+["'])(\.\/modules\/[^"']+)(["'])/g,
    `$1$2?v=${version}$3`,
  );
  await fs.writeFile(path.join(assetsDirectory, scriptFilename), versionedScript);

  return {
    script: `/assets/${scriptFilename}`,
    stylesheet: `/assets/${stylesheetFilename}`,
    version,
  };
};

const htmlEscape = (value = "") => String(value)
  .replaceAll("&", "&amp;")
  .replaceAll("<", "&lt;")
  .replaceAll(">", "&gt;")
  .replaceAll('"', "&quot;")
  .replaceAll("'", "&#039;");

const stripMarkdown = (value = "") => String(value)
  .replace(/```[\s\S]*?```/g, " ")
  .replace(/`([^`]+)`/g, "$1")
  .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
  .replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
  .replace(/<[^>]+>/g, " ")
  .replace(/[*_~>#|]/g, " ")
  .replace(/\s+/g, " ")
  .trim();

const slugify = (value = "") => stripMarkdown(value)
  .toLowerCase()
  .normalize("NFKD")
  .replace(/[\u0300-\u036f]/g, "")
  .replace(/[^a-z0-9]+/g, "-")
  .replace(/^-|-$/g, "") || "section";

const fileToSlug = (filename) => {
  if (filename.toLowerCase() === "readme.md") return "";
  return filename.replace(/\.md$/i, "").replace(/^\d+[\s_-]*/, "");
};

const getDescription = (markdown) => {
  const paragraph = markdown
    .split(/\n\s*\n/)
    .map((part) => part.trim())
    .find((part) => part && !/^(#|```|\||-|\*\s|<)/.test(part));
  return stripMarkdown(paragraph || "Documentation for remote.futrx.").slice(0, 180);
};

const compareNames = (left, right) => {
  if (left.toLowerCase() === "readme.md") return -1;
  if (right.toLowerCase() === "readme.md") return 1;
  return left.localeCompare(right, undefined, { numeric: true });
};

const sectionLabel = (directoryName) => directoryName
  .replace(/^\d+[\s_-]*/, "")
  .replace(/[-_]+/g, " ")
  .replace(/\b\w/g, (character) => character.toUpperCase());

const readSectionFiles = async (sectionDirectory, relativeDirectory = "") => {
  const absoluteDirectory = path.join(docsDirectory, sectionDirectory, relativeDirectory);
  const entries = await fs.readdir(absoluteDirectory, { withFileTypes: true });
  const files = [];

  for (const entry of entries.sort((left, right) => compareNames(left.name, right.name))) {
    if (entry.name.startsWith(".")) continue;
    const relativePath = path.posix.join(relativeDirectory, entry.name);
    if (entry.isDirectory()) {
      files.push(...await readSectionFiles(sectionDirectory, relativePath));
    } else if (entry.isFile() && entry.name.toLowerCase().endsWith(".md")) {
      files.push({
        filename: entry.name,
        relativePath: path.posix.join(sectionDirectory, relativePath),
        isRootDocument: false,
        nestedDirectory: path.posix.dirname(relativePath),
      });
    }
  }

  return files;
};

const documentRouteSegments = (file) => {
  const nestedSegments = file.nestedDirectory === "."
    ? []
    : file.nestedDirectory.split("/").map((segment) => slugify(sectionLabel(segment)));
  const documentSlug = fileToSlug(file.filename);
  return documentSlug ? [...nestedSegments, documentSlug] : nestedSegments;
};

const readDocuments = async () => {
  const entries = await fs.readdir(docsDirectory, { withFileTypes: true });
  const rootFiles = entries
    .filter((entry) => entry.isFile() && entry.name.toLowerCase().endsWith(".md"))
    .map((entry) => entry.name)
    .sort(compareNames);
  const directories = entries
    .filter((entry) => entry.isDirectory() && !entry.name.startsWith(".") && !excludedDocumentDirectories.has(entry.name))
    .map((entry) => entry.name)
    .sort(compareNames);
  const sectionSources = [];

  for (const directory of directories) {
    const files = await readSectionFiles(directory);
    if (!files.length) continue;
    const label = sectionLabel(directory);

    if (directory === homeDocumentDirectory) {
      files.push(...rootFiles.map((filename) => ({
        filename,
        relativePath: filename,
        isRootDocument: true,
        nestedDirectory: ".",
      })));
    }

    sectionSources.push({ directory, label, slug: slugify(label), files });
  }

  const documents = [];
  for (const [sectionOrder, source] of sectionSources.entries()) {
    const section = { ...source, order: sectionOrder, documents: [] };

    for (const [sectionDocumentOrder, file] of source.files.entries()) {
      const { filename, relativePath, isRootDocument } = file;
      const markdown = await fs.readFile(path.join(docsDirectory, relativePath), "utf8");
      const headingMatches = [...markdown.matchAll(/^(#{1,3})\s+(.+)$/gm)];
      const title = stripMarkdown(headingMatches.find((match) => match[1].length === 1)?.[2] || filename.replace(/\.md$/i, ""));
      const headingSlugs = new Map();
      const headings = headingMatches
        .filter((match) => match[1].length > 1)
        .map((match) => {
          const text = stripMarkdown(match[2]);
          const base = slugify(text);
          const count = headingSlugs.get(base) || 0;
          headingSlugs.set(base, count + 1);
          return { depth: match[1].length, text, id: count ? `${base}-${count + 1}` : base };
        });
      const document = {
        filename,
        relativePath,
        order: documents.length,
        sectionDocumentOrder,
        markdown,
        slug: fileToSlug(filename),
        routeSegments: documentRouteSegments(file),
        isRootDocument,
        title,
        description: getDescription(markdown),
        headings,
        section,
      };
      section.documents.push(document);
      documents.push(document);
    }
  }

  const homeDocument = documents.find((document) => document.relativePath === homeDocumentPath);
  if (!homeDocument) throw new Error(`Required home document not found: ${homeDocumentPath}`);
  homeDocument.isSiteHome = true;

  const routes = new Map();
  for (const document of documents) {
    const route = pagePath(document);
    if (routes.has(route)) {
      throw new Error(`Duplicate documentation route ${route}: ${routes.get(route)} and ${document.relativePath}`);
    }
    routes.set(route, document.relativePath);
  }

  return documents;
};

const pagePath = (document) => {
  if (document.isSiteHome) return "/";
  if (document.isRootDocument) return `/${document.slug}/`;
  const routeSegments = [document.section.slug, ...(document.routeSegments || [document.slug])]
    .filter(Boolean);
  return `/${routeSegments.join("/")}/`;
};

const safeDecodeURIComponent = (value) => {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
};

const resolveLink = (href, documents, currentDocument) => {
  let resolvedHref = href.trim();
  const pathPart = resolvedHref.split("#")[0] || "";
  const markdownTarget = safeDecodeURIComponent(pathPart);
  const hash = resolvedHref.includes("#") ? `#${resolvedHref.split("#").slice(1).join("#")}` : "";

  if (markdownTarget.toLowerCase().endsWith(".md")) {
    const targetPath = path.posix.normalize(path.posix.join(path.posix.dirname(currentDocument.relativePath), markdownTarget));
    const exactTarget = documents.find((document) => document.relativePath.toLowerCase() === targetPath.toLowerCase());
    const targetName = path.posix.basename(markdownTarget);
    const matchingNames = documents.filter((document) => document.filename.toLowerCase() === targetName.toLowerCase());
    const target = exactTarget || (matchingNames.length === 1 ? matchingNames[0] : undefined);
    if (target) {
      resolvedHref = `${pagePath(target)}${hash}`;
    } else if (resolvedHref.startsWith("../") || resolvedHref.startsWith("./")) {
      resolvedHref = "#";
    }
  } else if (resolvedHref.startsWith("../") || resolvedHref.startsWith("./")) {
    const documentPath = path.posix.normalize(path.posix.join(path.posix.dirname(currentDocument.relativePath), pathPart));
    resolvedHref = documentPath.startsWith("assets/")
      ? `/assets/docs/${documentPath.slice("assets/".length)}${hash}`
      : "#";
  }

  if (/^[a-z][a-z\d+.-]*:/i.test(resolvedHref) && !/^(https?:|mailto:)/i.test(resolvedHref)) return "#";
  return resolvedHref;
};

const isPrivateRepositoryHref = (href) => href === privateRepositoryUrl || href.startsWith(`${privateRepositoryUrl}/`);

const renderInline = (value, documents, currentDocument) => {
  const placeholders = [];
  const hold = (html) => `\u0000${placeholders.push(html) - 1}\u0000`;
  let text = String(value);

  text = text.replace(/!\[([^\]]*)\]\((\S+?)(?:\s+["']([^"']*)["'])?\)/g, (_, alt, source, title) => {
    const resolvedSource = resolveLink(source, documents, currentDocument);
    const titleAttribute = title ? ` title="${htmlEscape(title)}"` : "";
    return hold(`<img src="${htmlEscape(resolvedSource)}" alt="${htmlEscape(stripMarkdown(alt))}"${titleAttribute} loading="lazy" decoding="async">`);
  });

  text = text.replace(/\[([^\]]+)\]\((\S+?)(?:\s+["']([^"']*)["'])?\)/g, (_, label, href, title) => {
    const resolvedHref = resolveLink(href, documents, currentDocument);
    const renderedLabel = renderInline(label, documents, currentDocument);
    if (resolvedHref === "#" || isPrivateRepositoryHref(resolvedHref)) {
      const reason = isPrivateRepositoryHref(resolvedHref)
        ? "Source repository is currently private"
        : "This source link is unavailable in the public documentation";
      return hold(`<span class="disabled-link" title="${reason}">${renderedLabel}</span>`);
    }
    const titleAttribute = title ? ` title="${htmlEscape(title)}"` : "";
    const external = /^https?:\/\//.test(resolvedHref) ? ' target="_blank" rel="noreferrer"' : "";
    return hold(`<a href="${htmlEscape(resolvedHref)}"${titleAttribute}${external}>${renderedLabel}</a>`);
  });

  text = text.replace(/`([^`\n]+)`/g, (_, code) => hold(`<code>${htmlEscape(code)}</code>`));
  text = htmlEscape(text);
  text = text
    .replace(/\*\*([^*\n]+)\*\*/g, "<strong>$1</strong>")
    .replace(/__([^_\n]+)__/g, "<strong>$1</strong>")
    .replace(/~~([^~\n]+)~~/g, "<del>$1</del>")
    .replace(/(^|[^*])\*([^*\n]+)\*(?!\*)/g, "$1<em>$2</em>")
    .replace(/(^|[^_])_([^_\n]+)_(?!_)/g, "$1<em>$2</em>");

  return text.replace(/\u0000(\d+)\u0000/g, (_, index) => placeholders[Number(index)]);
};

const renderCodeBlock = (text, language = "") => {
  const label = language.trim().split(/\s+/)[0] || "text";
  if (label === "mermaid") {
    return `<div class="mermaid-shell"><div class="mermaid">${htmlEscape(text)}</div></div>`;
  }
  return `<div class="code-block"><div class="code-bar"><span>${htmlEscape(label)}</span><button type="button" data-copy-code>Copy</button></div><pre><code class="language-${htmlEscape(label)}">${htmlEscape(text)}</code></pre></div>`;
};

const splitTableRow = (line) => line
  .trim()
  .replace(/^\|/, "")
  .replace(/\|$/, "")
  .split("|")
  .map((cell) => cell.trim());

const isTableDivider = (line = "") => {
  const cells = splitTableRow(line);
  return cells.length > 0 && cells.every((cell) => /^:?-{3,}:?$/.test(cell));
};

const listMatch = (line = "") => line.match(/^(\s*)([-+*]|\d+[.)])\s+(.+)$/);

const startsBlock = (lines, index) => {
  const line = lines[index] || "";
  if (/^ {0,3}```/.test(line)) return true;
  if (/^ {0,3}#{1,6}\s+/.test(line)) return true;
  if (/^ {0,3}>/.test(line)) return true;
  if (/^ {0,3}((\*\s*){3,}|(-\s*){3,}|(_\s*){3,})$/.test(line)) return true;
  if (listMatch(line)) return true;
  return line.includes("|") && isTableDivider(lines[index + 1]);
};

const renderMarkdown = (markdown, documents, currentDocument) => {
  const lines = String(markdown).replaceAll("\r\n", "\n").split("\n");
  const headingCounts = new Map();
  const output = [];
  let index = 0;

  while (index < lines.length) {
    const line = lines[index];

    if (!line.trim()) {
      index += 1;
      continue;
    }

    const fence = line.match(/^ {0,3}```\s*([^`]*)$/);
    if (fence) {
      const code = [];
      index += 1;
      while (index < lines.length && !/^ {0,3}```\s*$/.test(lines[index])) {
        code.push(lines[index]);
        index += 1;
      }
      if (index < lines.length) index += 1;
      output.push(renderCodeBlock(code.join("\n"), fence[1]));
      continue;
    }

    const standaloneImage = line.match(/^!\[([^\]]*)\]\((\S+?)(?:\s+["']([^"']*)["'])?\)\s*$/);
    if (standaloneImage) {
      const [, alt, source, title] = standaloneImage;
      const resolvedSource = resolveLink(source, documents, currentDocument);
      const cleanAlt = stripMarkdown(alt);
      const caption = stripMarkdown(title || alt);
      const titleAttribute = title ? ` title="${htmlEscape(title)}"` : "";
      const captionElement = caption ? `<figcaption>${htmlEscape(caption)}</figcaption>` : "";
      output.push(`<figure class="doc-figure"><img src="${htmlEscape(resolvedSource)}" alt="${htmlEscape(cleanAlt)}"${titleAttribute} loading="lazy" decoding="async">${captionElement}</figure>`);
      index += 1;
      continue;
    }

    const heading = line.match(/^ {0,3}(#{1,6})\s+(.+?)\s*#*\s*$/);
    if (heading) {
      const depth = heading[1].length;
      const plainText = stripMarkdown(heading[2]);
      const base = slugify(plainText);
      const count = headingCounts.get(base) || 0;
      headingCounts.set(base, count + 1);
      const id = count ? `${base}-${count + 1}` : base;
      output.push(`<h${depth} id="${htmlEscape(id)}">${renderInline(heading[2], documents, currentDocument)}<a class="heading-link" href="#${htmlEscape(id)}" aria-label="Link to ${htmlEscape(plainText)}">#</a></h${depth}>`);
      index += 1;
      continue;
    }

    if (/^ {0,3}((\*\s*){3,}|(-\s*){3,}|(_\s*){3,})$/.test(line)) {
      output.push("<hr>");
      index += 1;
      continue;
    }

    if (line.includes("|") && isTableDivider(lines[index + 1])) {
      const headers = splitTableRow(line);
      const alignments = splitTableRow(lines[index + 1]).map((cell) => cell.startsWith(":") && cell.endsWith(":") ? "center" : cell.endsWith(":") ? "right" : "left");
      const rows = [];
      index += 2;
      while (index < lines.length && lines[index].includes("|") && lines[index].trim()) {
        rows.push(splitTableRow(lines[index]));
        index += 1;
      }
      output.push(`<table><thead><tr>${headers.map((cell, cellIndex) => `<th style="text-align:${alignments[cellIndex] || "left"}">${renderInline(cell, documents, currentDocument)}</th>`).join("")}</tr></thead><tbody>${rows.map((row) => `<tr>${headers.map((_, cellIndex) => `<td style="text-align:${alignments[cellIndex] || "left"}">${renderInline(row[cellIndex] || "", documents, currentDocument)}</td>`).join("")}</tr>`).join("")}</tbody></table>`);
      continue;
    }

    if (/^ {0,3}>/.test(line)) {
      const quote = [];
      while (index < lines.length && /^ {0,3}>/.test(lines[index])) {
        quote.push(lines[index].replace(/^ {0,3}>\s?/, ""));
        index += 1;
      }
      output.push(`<blockquote>${renderMarkdown(quote.join("\n"), documents, currentDocument)}</blockquote>`);
      continue;
    }

    const firstListItem = listMatch(line);
    if (firstListItem) {
      const ordered = /^\d/.test(firstListItem[2]);
      const indentation = firstListItem[1].length;
      const items = [];
      const start = ordered ? Number.parseInt(firstListItem[2], 10) : 1;

      while (index < lines.length) {
        const item = listMatch(lines[index]);
        if (!item || item[1].length !== indentation || /^\d/.test(item[2]) !== ordered) break;
        const parts = [item[3]];
        index += 1;
        while (index < lines.length && lines[index].trim() && !listMatch(lines[index]) && /^\s+/.test(lines[index])) {
          parts.push(lines[index].trim());
          index += 1;
        }
        items.push(parts.join(" "));
      }

      const tag = ordered ? "ol" : "ul";
      const startAttribute = ordered && start !== 1 ? ` start="${start}"` : "";
      output.push(`<${tag}${startAttribute}>${items.map((item) => `<li>${renderInline(item, documents, currentDocument)}</li>`).join("")}</${tag}>`);
      continue;
    }

    const paragraph = [line.trim()];
    index += 1;
    while (index < lines.length && lines[index].trim() && !startsBlock(lines, index)) {
      paragraph.push(lines[index].trim());
      index += 1;
    }
    output.push(`<p>${renderInline(paragraph.join(" "), documents, currentDocument)}</p>`);
  }

  return output.join("\n");
};

const topNavigation = (documents, current) => {
  const sections = [...new Map(documents.map((document) => [document.section.order, document.section])).values()];
  return `
    <nav class="top-navigation" aria-label="Documentation sections">
      ${sections.map((section) => `
        <a class="top-navigation-link${section.order === current.section.order ? " is-active" : ""}" href="${pagePath(section.documents[0])}"${section.order === current.section.order ? ' aria-current="page"' : ""}>${htmlEscape(section.label)}</a>`).join("")}
    </nav>`;
};

const navigation = (current) => `
  <p class="sidebar-label">${htmlEscape(current.section.label)}</p>
  <nav class="sidebar-nav" aria-label="${htmlEscape(current.section.label)} pages">
    ${current.section.documents.map((document) => `
      <a class="sidebar-link${document.relativePath === current.relativePath ? " is-active" : ""}" href="${pagePath(document)}"${document.relativePath === current.relativePath ? ' aria-current="page"' : ""}>
        <span>${htmlEscape(document.title)}</span>
      </a>`).join("")}
  </nav>`;

const tableOfContents = (document) => {
  const headings = document.headings.filter((heading) => heading.depth === 2);
  if (!headings.length) return "";
  return `
    <aside class="toc" aria-label="On this page">
      <p class="toc-title">On this page</p>
      <nav>${headings.map((heading) => `<a href="#${htmlEscape(heading.id)}">${htmlEscape(heading.text)}</a>`).join("")}</nav>
    </aside>`;
};

const pageTemplate = ({ document, documents, content, staticAssets }) => {
  const previous = document.section.documents[document.sectionDocumentOrder - 1];
  const next = document.section.documents[document.sectionDocumentOrder + 1];
  const description = document.description || "Documentation for remote.futrx.";

  return `<!doctype html>
<html lang="en" data-theme="dark">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <meta name="description" content="${htmlEscape(description)}">
    <meta name="theme-color" content="#0f1014">
    <title>${htmlEscape(document.title)} · remote.futrx docs</title>
    <link rel="icon" href="/assets/brand/remote-futrx-mark.png" type="image/png">
    <link rel="apple-touch-icon" href="/assets/brand/remote-futrx-mark.png">
    <link rel="stylesheet" href="${staticAssets.stylesheet}">
    <script>try{document.documentElement.dataset.theme=localStorage.getItem("docs-theme")||"dark"}catch{}</script>
    <script type="module" src="${staticAssets.script}"></script>
  </head>
  <body>
    <a class="skip-link" href="#main">Skip to content</a>
    <header class="site-header">
      <div class="header-inner">
        <button class="mobile-menu" type="button" data-nav-toggle aria-label="Open navigation" aria-expanded="false"><span></span><span></span><span></span></button>
        <a class="brand" href="https://remote.futrx.com/" aria-label="Visit the Remote by FutrX main site">
          <span class="brand-logo-surface">
            <img class="brand-logo" src="/assets/brand/remote-futrx-on-dark.png" width="720" height="254" alt="">
          </span>
          <span class="brand-tag">Docs</span>
        </a>
        <button class="search-trigger" type="button" data-search-open>
          <span class="search-symbol" aria-hidden="true"></span>
          <span>Search docs</span>
          <kbd>⌘ K</kbd>
        </button>
        <div class="header-actions">
          <button class="theme-toggle" type="button" data-theme-toggle aria-label="Toggle color theme"><span aria-hidden="true">◐</span></button>
        </div>
      </div>
      ${topNavigation(documents, document)}
    </header>

    <div class="nav-scrim" data-nav-close></div>
    <div class="docs-layout">
      <aside class="sidebar" data-sidebar>
        <div class="sidebar-mobile-head"><strong>Browse docs</strong><button type="button" data-nav-close aria-label="Close navigation">×</button></div>
        ${navigation(document)}
        <div class="sidebar-foot">
          <span class="status-dot" aria-hidden="true"></span>
          Generated from Markdown
        </div>
      </aside>

      <main class="main" id="main">
        <article class="article">
          <div class="article-tools">
            <nav class="breadcrumbs" aria-label="Breadcrumb"><a href="/">Docs</a><span>/</span><a href="${pagePath(document.section.documents[0])}">${htmlEscape(document.section.label)}</a><span>/</span><span>${htmlEscape(document.title)}</span></nav>
          </div>
          <div class="doc-content">${content}</div>
          <nav class="page-nav" aria-label="Previous and next pages">
            ${previous ? `<a class="page-nav-link previous" href="${pagePath(previous)}"><span>Previous</span><strong>← ${htmlEscape(previous.title)}</strong></a>` : "<span></span>"}
            ${next ? `<a class="page-nav-link next" href="${pagePath(next)}"><span>Next</span><strong>${htmlEscape(next.title)} →</strong></a>` : "<span></span>"}
          </nav>
          <footer class="article-footer">remote.futrx documentation</footer>
        </article>
      </main>

      ${tableOfContents(document)}
    </div>

    <dialog class="search-dialog" data-search-dialog>
      <div class="search-box">
        <div class="search-input-row"><span class="search-symbol" aria-hidden="true"></span><input type="search" placeholder="Search documentation…" aria-label="Search documentation" data-search-input><button type="button" data-search-close aria-label="Close search">Esc</button></div>
        <div class="search-results" data-search-results><p>Start typing to search all documentation.</p></div>
        <div class="search-footer"><span><kbd>↑</kbd> <kbd>↓</kbd> Navigate</span><span><kbd>Enter</kbd> Open</span></div>
      </div>
    </dialog>
  </body>
</html>`;
};

const notFoundTemplate = (staticAssets) => `<!doctype html>
<html lang="en" data-theme="dark">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <meta name="robots" content="noindex">
    <meta name="theme-color" content="#0f1014">
    <title>Page not found · remote.futrx docs</title>
    <link rel="icon" href="/assets/brand/remote-futrx-mark.png" type="image/png">
    <link rel="apple-touch-icon" href="/assets/brand/remote-futrx-mark.png">
    <link rel="stylesheet" href="${staticAssets.stylesheet}">
    <script>try{document.documentElement.dataset.theme=localStorage.getItem("docs-theme")||"dark"}catch{}</script>
  </head>
  <body class="not-found-page">
    <a class="skip-link" href="#main">Skip to content</a>
    <main class="not-found-shell" id="main">
      <a class="brand" href="https://remote.futrx.com/" aria-label="Visit the Remote by FutrX main site">
        <span class="brand-logo-surface">
          <img class="brand-logo" src="/assets/brand/remote-futrx-on-dark.png" width="720" height="254" alt="">
        </span>
        <span class="brand-tag">Docs</span>
      </a>
      <p class="not-found-code">404</p>
      <h1>That page isn't here.</h1>
      <p>The address may have changed, or the page may no longer be part of the public documentation.</p>
      <a class="not-found-link" href="/">Return to the documentation</a>
    </main>
  </body>
</html>`;

const searchIndexFor = (documents) => documents.map((document) => ({
  title: document.title,
  url: pagePath(document),
  description: document.description,
  headings: document.headings.map((heading) => heading.text),
  content: stripMarkdown(document.markdown).slice(0, 24000),
}));

const manifestFor = (documents) => documents.map((document) => ({
  section: document.section.label,
  source: document.relativePath,
  title: document.title,
  url: pagePath(document),
}));

const build = async () => {
  const documents = await readDocuments();
  if (!documents.length) throw new Error(`No Markdown files found in ${docsDirectory}`);

  await fs.rm(outputDirectory, { recursive: true, force: true });
  await fs.mkdir(outputDirectory, { recursive: true });
  const staticAssets = await writeStaticAssets();
  const docsAssets = await fs.stat(docsAssetsDirectory).catch(() => null);
  if (docsAssets?.isDirectory()) {
    await fs.cp(docsAssetsDirectory, path.join(outputDirectory, "assets", "docs"), { recursive: true });
  }
  await fs.mkdir(path.join(outputDirectory, "_source"), { recursive: true });

  for (const document of documents) {
    const content = renderMarkdown(document.markdown, documents, document);
    const outputPath = pagePath(document).replace(/^\/|\/$/g, "");
    const pageDirectory = outputPath ? path.join(outputDirectory, outputPath) : outputDirectory;
    const sourcePath = path.join(outputDirectory, "_source", document.relativePath);
    await fs.mkdir(pageDirectory, { recursive: true });
    await fs.writeFile(path.join(pageDirectory, "index.html"), pageTemplate({ document, documents, content, staticAssets }));
    await fs.mkdir(path.dirname(sourcePath), { recursive: true });
    await fs.copyFile(path.join(docsDirectory, document.relativePath), sourcePath);
  }

  const searchIndex = searchIndexFor(documents);

  await fs.writeFile(path.join(outputDirectory, "search-index.json"), JSON.stringify(searchIndex));
  await fs.writeFile(path.join(outputDirectory, "404.html"), notFoundTemplate(staticAssets));
  await fs.writeFile(path.join(outputDirectory, "docs-manifest.json"), JSON.stringify(manifestFor(documents), null, 2));
  await fs.writeFile(path.join(outputDirectory, "robots.txt"), "User-agent: *\nAllow: /\nSitemap: https://docs.remote.futrx.com/sitemap.xml\n");
  await fs.writeFile(path.join(outputDirectory, "sitemap.xml"), `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${documents.map((document) => `<url><loc>https://docs.remote.futrx.com${pagePath(document)}</loc></url>`).join("")}</urlset>\n`);
  await fs.writeFile(path.join(outputDirectory, "_headers"), `/*\n  X-Content-Type-Options: nosniff\n  Referrer-Policy: strict-origin-when-cross-origin\n  Permissions-Policy: camera=(), microphone=(), geolocation=()\n\n/assets/*\n  Cache-Control: public, max-age=3600\n`);

  console.log(`Built ${documents.length} Markdown pages into dist/ with assets ${staticAssets.version}`);
};

const mimeTypes = {
  ".css": "text/css; charset=utf-8",
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".md": "text/markdown; charset=utf-8",
  ".txt": "text/plain; charset=utf-8",
  ".xml": "application/xml; charset=utf-8",
};

const serve = async () => {
  const port = Number(process.env.PORT || 4173);
  const server = createServer(async (request, response) => {
    try {
      const requestPath = decodeURIComponent(new URL(request.url, `http://${request.headers.host}`).pathname);
      let filePath = path.join(outputDirectory, requestPath);
      const stat = await fs.stat(filePath).catch(() => null);
      if (stat?.isDirectory()) filePath = path.join(filePath, "index.html");
      if (!stat && !path.extname(filePath)) filePath = path.join(filePath, "index.html");
      const contents = await fs.readFile(filePath);
      response.writeHead(200, { "Content-Type": mimeTypes[path.extname(filePath)] || "application/octet-stream" });
      response.end(contents);
    } catch {
      response.writeHead(404, { "Content-Type": "text/plain; charset=utf-8" });
      response.end("Not found");
    }
  });

  server.listen(port, "127.0.0.1", () => console.log(`Local: http://localhost:${port}/`));

  let rebuildTimer;
  const scheduleBuild = () => {
    clearTimeout(rebuildTimer);
    rebuildTimer = setTimeout(() => build().catch(console.error), 120);
  };
  watch(docsDirectory, { recursive: true }, scheduleBuild);
  watch(staticDirectory, { recursive: true }, scheduleBuild);
};

const testMarkdownRenderer = () => {
  const testSection = { slug: "", documents: [] };
  const testDocuments = [{ filename: "guide.md", relativePath: "guide.md", slug: "guide", section: testSection }];
  testSection.documents = testDocuments;
  const rendered = renderMarkdown(`# Hello world

Text with \`inline code\`, **strong text**, and a [guide](guide.md).

| Name | Value |
| --- | ---: |
| Alpha | One |

- First
- Second

\`\`\`html
<script>alert("no")</script>
\`\`\`

<img src=x onerror=alert("no")>

[Unsafe](javascript:alert)`, testDocuments, testDocuments[0]);

  assert.match(rendered, /<h1 id="hello-world">/);
  assert.match(rendered, /<code>inline code<\/code>/);
  assert.match(rendered, /<strong>strong text<\/strong>/);
  assert.match(rendered, /href="\/guide\/"/);
  assert.match(rendered, /<table>/);
  assert.match(rendered, /<ul><li>First<\/li><li>Second<\/li><\/ul>/);
  assert.match(rendered, /&lt;script&gt;alert\(&quot;no&quot;\)&lt;\/script&gt;/);
  assert.match(rendered, /&lt;img src=x onerror=alert\(&quot;no&quot;\)&gt;/);
  assert.match(rendered, /class="disabled-link"/);
  assert.doesNotMatch(rendered, /<script>|<img src=x/);

  const media = renderMarkdown(`![Remote workspace](/assets/docs/workspace.png "An isolated Remote workspace")

[Private source](${privateRepositoryUrl}/blob/main/README.md)`, testDocuments, testDocuments[0]);
  assert.match(media, /<figure class="doc-figure">/);
  assert.match(media, /src="\/assets\/docs\/workspace\.png"/);
  assert.match(media, /<figcaption>An isolated Remote workspace<\/figcaption>/);
  assert.match(media, /<span class="disabled-link"/);
  assert.doesNotMatch(media, /href="https:\/\/github\.com\/futrx-com\/remote\.futrx\.com/);

  const nestedDocument = { ...testDocuments[0], relativePath: "01-overview/guide.md" };
  const relativeMedia = renderMarkdown(`![Preview](../assets/screenshots/preview.webp "Live preview")

[Architecture](../../ARCHITECTURE.md)`, testDocuments, nestedDocument);
  assert.match(relativeMedia, /src="\/assets\/docs\/screenshots\/preview\.webp"/);
  assert.match(relativeMedia, /<span class="disabled-link"[^>]*>Architecture<\/span>/);
  console.log("Markdown renderer tests passed");
};

const testDocumentModel = async () => {
  const documents = await readDocuments();
  const homeDocument = documents.find((document) => document.isSiteHome);
  const sections = [...new Map(documents.map((document) => [document.section.order, document.section])).values()];

  assert.equal(homeDocument?.relativePath, homeDocumentPath);
  assert.equal(pagePath(homeDocument), "/");
  assert.equal(sections.filter((section) => section.label === "Overview").length, 1);
  assert.equal(documents.find((document) => document.relativePath === "known-limitations.md") && pagePath(documents.find((document) => document.relativePath === "known-limitations.md")), "/known-limitations/");
  assert.equal(documents.some((document) => document.relativePath.startsWith("codex-analysis/")), false);
  assert.equal(documents.some((document) => document.relativePath.startsWith("fable-analysis/")), false);

  const agentGuide = documents.find((document) => document.relativePath === "dev/agents/README.md");
  const addingAgent = documents.find((document) => document.relativePath === "dev/agents/07-adding-an-agent.md");
  const pushGuide = documents.find((document) => document.relativePath === "dev/push-notifications/README.md");
  assert.ok(agentGuide);
  assert.ok(addingAgent);
  assert.ok(pushGuide);
  assert.equal(pagePath(agentGuide), "/dev/agents/");
  assert.equal(pagePath(addingAgent), "/dev/agents/adding-an-agent/");
  assert.equal(pagePath(pushGuide), "/dev/push-notifications/");

  assert.deepEqual(
    manifestFor(documents).find((entry) => entry.source === agentGuide.relativePath),
    {
      section: "Dev",
      source: "dev/agents/README.md",
      title: "Agent integration developer guide",
      url: "/dev/agents/",
    },
  );
  assert.equal(
    searchIndexFor(documents).find((entry) => entry.url === "/dev/agents/")?.title,
    "Agent integration developer guide",
  );

  const overview = documents.find((document) => document.relativePath === "01-overview/README.md");
  assert.ok(overview);
  const overviewGuideLink = renderMarkdown("[Agent guide](../dev/agents/README.md)", documents, overview);
  const localGuideLink = renderMarkdown("[Add one](07-adding-an-agent.md)", documents, agentGuide);
  const crossSectionLink = renderMarkdown(
    "[Update flow](../../04-operations/09-deployment-and-operations.md#update-flow)",
    documents,
    addingAgent,
  );
  assert.match(overviewGuideLink, /href="\/dev\/agents\/"/);
  assert.match(localGuideLink, /href="\/dev\/agents\/adding-an-agent\/"/);
  assert.match(crossSectionLink, /href="\/operations\/deployment-and-operations\/#update-flow"/);

  const staticAssets = { stylesheet: "/assets/site.test.css", script: "/assets/site.test.js" };
  const homeHtml = pageTemplate({ document: homeDocument, documents, content: "", staticAssets });
  const notFoundHtml = notFoundTemplate(staticAssets);
  for (const html of [homeHtml, notFoundHtml]) {
    assert.match(html, /href="https:\/\/remote\.futrx\.com\/"/);
    assert.match(html, /src="\/assets\/brand\/remote-futrx-on-dark\.png"/);
    assert.match(html, /href="\/assets\/brand\/remote-futrx-mark\.png"/);
  }
  console.log("Documentation model tests passed");
};

if (process.argv.includes("--test")) {
  testMarkdownRenderer();
  await testDocumentModel();
} else {
  await build();
  if (process.argv.includes("--serve")) await serve();
}

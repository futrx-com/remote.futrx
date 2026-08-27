# remote.futrx documentation site

A small static documentation site generated from the public Markdown files in `../docs`.

## Local development

```bash
npm install
npm run dev
```

The development server runs at `http://localhost:4173`. Changes to Markdown, CSS, or JavaScript trigger a rebuild.

## Build

```bash
npm run build
```

The complete static site is written to `dist/`. There is no framework, server, database, runtime API, or npm dependency. The build script includes a small Markdown renderer for the syntax used by these docs.

Run the dependency-free renderer and publication-model tests before building:

```bash
npm test
```

## Content and navigation

`../docs/01-overview/README.md` is always the home page. First-level
documentation folders become top-navigation sections, while Markdown files
below them are discovered recursively and become sidebar pages. Nested folders
remain in the generated URL, so `../docs/dev/agents/README.md` is published at
`/dev/agents/`. Numeric prefixes control ordering but are removed from labels
and URLs.

Root-level Markdown files are included at root URLs and grouped into the Overview sidebar. Internal analysis directories (`codex-analysis` and `fable-analysis`) and the `assets` directory are excluded from documentation discovery.

Place documentation images in `../docs/assets/`. The build copies that directory to `dist/assets/docs/`, so Markdown can reference an image as:

```markdown
![Project workspace](/assets/docs/project-workspace.png "An isolated project workspace")
```

A standalone image renders as a responsive figure. Its optional title becomes the visible caption; when no title is supplied, the alt text is used.

## Cloudflare Pages

The existing Cloudflare Pages project uses Direct Upload:

- Project: `remote-futrx-docs`
- Production branch: `main`
- Build output directory: `dist`
- Node.js version: `22`

Validate the generated manifest before publishing:

```bash
npm test
npm run build
jq -r '.[] | [.section,.source,.url] | @tsv' dist/docs-manifest.json
cf-use futrx
npm run deploy
```

The deploy script uses a pinned, temporary Wrangler CLI so the site itself remains dependency-free.

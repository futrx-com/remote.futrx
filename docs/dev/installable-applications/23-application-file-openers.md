# Workspace file openers

## Opening a workspace file through an extension

[`remote.files.registerOpener`](06-extension-api.md)
registers a synchronous `(request) => string | null` callback. The request has
`cwd`, `path`, and optional positive `line` and `column` numbers. The application
builds its URL and handles editor-specific file-position syntax.

The extension host keeps the opener's allowed project IDs in sync with running
installs and removes it on unload. A global install alone does not supply a
project ID, so it does not create a global file opener. One callback is kept per
application. Registering another replaces it. Across eligible applications,
the first nonempty returned URL wins in registry iteration order; there is no
priority field or user-facing editor selection. Returning `null` or throwing
allows the next opener to be tried; exceptions are logged.

The optional [File Manager application](../../../applications/file-manager/README.md)
uses `remote.files.open` with the drawer's project and chat context. Supported
media opens in the shared viewer, text/code files use a matching opener, and
other files download. Registering only an opener is sufficient; no header
button is required.

Markdown and attachments use the same opener registry independently of File
Manager. The workspace-link parser carries `:line[:column]`, translates eligible
host paths into `/workspace/...`, and rejects paths outside the workspace.
The destination HTTP route enforces filesystem containment and membership.

## Source and verification

- [applications/file-manager/](../../../applications/file-manager/)
- [frontend/src/services/files/openWorkspaceFile.ts](../../../frontend/src/services/files/openWorkspaceFile.ts)
- [frontend/src/app/containers/ChatContainer.tsx](../../../frontend/src/app/containers/ChatContainer.tsx)
- [frontend/src/app/extensions/extensionApi.ts](../../../frontend/src/app/extensions/extensionApi.ts)
- [frontend/src/app/extensions/extensionHost.ts](../../../frontend/src/app/extensions/extensionHost.ts)
- [frontend/src/config/workspace.ts](../../../frontend/src/config/workspace.ts)
- [frontend/src/models/extension.ts](../../../frontend/src/models/extension.ts)
- [frontend/src/models/files.ts](../../../frontend/src/models/files.ts)
- [frontend/src/services/files/fileService.test.ts](../../../frontend/src/services/files/fileService.test.ts)
- [frontend/src/services/files/fileService.ts](../../../frontend/src/services/files/fileService.ts)
- [frontend/src/services/files/workspaceLinkService.test.ts](../../../frontend/src/services/files/workspaceLinkService.test.ts)
- [frontend/src/services/files/workspaceLinkService.ts](../../../frontend/src/services/files/workspaceLinkService.ts)
- [frontend/src/state/hooks/chat/useWorkspaceFileUrl.ts](../../../frontend/src/state/hooks/chat/useWorkspaceFileUrl.ts)
- [frontend/src/ui/chat/header/WorkspaceActions.tsx](../../../frontend/src/ui/chat/header/WorkspaceActions.tsx)
- [frontend/src/ui/chat/markdown/Markdown.tsx](../../../frontend/src/ui/chat/markdown/Markdown.tsx)
- [frontend/src/ui/chat/markdown/inlineParser.tsx](../../../frontend/src/ui/chat/markdown/inlineParser.tsx)
- [frontend/src/ui/chat/messages/AttachmentPreviews.tsx](../../../frontend/src/ui/chat/messages/AttachmentPreviews.tsx)

The included unit tests verify decisions and generated requests/commands.
No live container installation, authenticated browser flow, or QA deployment
was performed for this split.

## What to verify

Tests check media/application/download classification, registered file-link
resolution and download fallback. Multiple opener
ordering/replacement is documented from implementation review rather than
exhaustively tested. Verify Markdown, attachments and Files while switching
projects; test an opener-only extension in the Files drawer separately.

### Responsibility boundaries

- [fileOpenerStore.ts](../../../frontend/src/state/stores/files/fileOpenerStore.ts) — Owns registration, project visibility, subscription notifications, and disposal.
- [resolveFileOpener.ts](../../../frontend/src/services/files/resolveFileOpener.ts) — Tries visible openers in registration order and logs callback errors before trying the next opener.

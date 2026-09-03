# Complete feature reference

This is the compact inventory of current Remote behavior. “Page” means the loaded browser application; “host” means the Remote server; “project” means the durable workspace and its LXD container.

## Navigation and project list

| Feature | How to use it | Availability and persistence |
| --- | --- | --- |
| Expand or collapse sidebar | Use the chevron in the sidebar header | Persists in browser local storage |
| Mobile sidebar | Use the menu button; tap outside or close to dismiss | Responsive UI |
| Search projects and chats | Type in **Search projects and chats**; clear with **×** | Filters names and visible groups |
| Project and chat counts | Read the summary below search | Live workspace state |
| Create project | Choose the blue **+**, enter a name | Any registered user; project persists |
| Reorder projects | Drag a project group when no search is active | Saved project order |
| Expand project group | Use the project chevron | Per loaded page |
| Inspect project | Use the gear beside the project name | Members and admins |
| Create project chat | Use the **+** beside a provisioned project | Members and admins |
| Provisioning and errors | Read the disabled/spinning new-chat control or project error text | Live workspace updates |
| Chat model and time | Read each chat row's model badge and relative time | Persisted metadata |
| Running indicator | Spinner beside an active chat | Live run state |
| Unread indicator | Green dot beside completed unseen work | Persisted `lastReadAt` |
| Mark read or unread | Hover a chat and use the eye control | Persisted |
| Fork chat | Hover a chat and use the fork control | New persisted chat |
| Delete chat | Hover a chat and use **×**, then confirm | Removes chat metadata/history |
| Loose chats | Appear under **Unassigned** | Run on the host and are outside project isolation; avoid |
| Account settings and sign-out | Use the account footer | Current user/session |

## Chat composer

| Feature | How to use it | Important behavior |
| --- | --- | --- |
| Provider | Choose **Codex**, **MiniMax**, **Claude**, **Kimi**, or **Antigravity** | MiniMax is project-only; cannot change while streaming |
| Model | Open the provider/model picker and select a discovered model or Auto | Stored per chat; choices come from the current host/project CLI catalog |
| Refresh models | Use the refresh action at the bottom of the provider/model picker | Force-probes the current scope; use after CLI, configuration, account, entitlement, or terminal-login changes |
| Thinking | Select one of the efforts reported for the current provider/model | Hidden when no effort control is advertised; Kimi currently stores but does not forward the selection |
| Speed | Select a service tier reported for the current provider/model | Codex tiers and eligible Claude Fast are supported; account/provider may gate them |
| Mode | Choose Default or provider-native Plan | Hidden when Plan is unavailable |
| Skill picker | Open **Skill set**, search, and select | Catalog depends on provider/project |
| Skill chips | Review or remove selected skills | Cleared when provider changes |
| Attach picker | Choose **+** and select one or more files | Project chats; resumable uploads |
| Drag attachment | Drag files over the composer | Same upload path |
| Paste image | Paste image clipboard data into the composer | Same upload path |
| Attachment chips | Monitor or remove before sending | Saved file path is added to prompt |
| Prompt text | Type the instruction; Shift+Enter adds a line | Draft survives chat switching and same-tab reloads |
| Send | Press Enter or use the arrow | One active run per chat |
| Queue | Send while streaming | Per-tab `sessionStorage`; not a server-side job |
| Remove queued prompt | Use **×** on a queue chip | Before it auto-sends |
| Cancel | Use the red square or Escape while running | Cancels active provider context |

The placeholder mentions `@` files and `/` commands, but the current source has no implemented mention or slash-command picker. Use the attachment control and skill picker instead.

## Conversation and messages

| Feature | Visible behavior |
| --- | --- |
| Streaming text | Assistant output appears incrementally |
| Reasoning | Consecutive provider reasoning deltas are grouped into collapsed, live-updating blocks that can be expanded at any time |
| Tool groups | Consecutive tools are grouped and expandable |
| Specialized tools | Read, write, edit, search, shell, and questions have tailored cards |
| Generic tools | Unknown tools use a generic renderer |
| Markdown | Headings, lists, links, tables, blockquotes, and code render in messages |
| Syntax highlighting | Code fences receive language-aware highlighting |
| AskUserQuestion | Agent questions become a paged single/multi-select form with **Other** |
| Usage | Supported providers report accumulated token usage |
| Working state | Header dot, provider label, sidebar spinner, and composer state update |
| Load older | Complete older conversation turns page backward |
| Jump to latest | Appears when reading above the newest output |
| Reconnect/replay | Chat socket resumes from the last event sequence |
| Automatic title | Remote can title a new chat from its early content; it is persisted, and the current UI has no manual rename control |
| Rewind | Deletes the selected point and later events; next run starts fresh |
| Fork | Copies visible history; provider-specific session fork happens on next run |
| Error block | Run and transport failures render in the thread |
| Schedules drawer | Project-chat header lists, edits, arms, pauses, runs, and deletes scheduled tasks |

There is no approval workflow in the current chat transport. Project agents run with provider approval/sandbox bypasses inside the project container.

## Providers and current differences

| Capability | Claude | Codex | MiniMax | Kimi | Antigravity |
| --- | ---: | ---: | ---: | ---: | ---: |
| Sign-in | Host authorization URL and pasted code | Host device flow | Host-managed write-only API-key form | Host device flow | Run `agy` in each project Terminal |
| Model picker | Live `/model` list with attempted version resolution | Live paginated app-server list | Provider-owned `MiniMax-M3` catalog | Configured models from the provider catalog | Models/variants returned by signed-in `agy` |
| Thinking control | Forwarded | Forwarded | Think-Off or Adaptive | Displayed/stored per model, not yet forwarded | Forwarded as Auto, Low, Medium, or High |
| Speed/service tier | Fast for Auto and Opus | Yes | No | No | No |
| Plan mode | Declared native mode | Discovered app-server mode | Codex-harness native mode | Advertised but incompatible with Remote prompt mode in the currently pinned Kimi CLI | Discovered native mode |
| Usage telemetry | Yes | Yes | Yes | No | No |
| Provider session fork | Yes | Yes, native app-server fork | Yes, native app-server fork | No; starts fresh | No; starts fresh |
| Selected skill trigger | Slash command | Dollar mention | Dollar mention | Canonical `SKILL.md` instruction | Canonical `SKILL.md` instruction |
| Browser MCP | Yes | Yes | Yes | No equivalent plumbing | No equivalent plumbing |
| Structured tool stream | Yes | Yes | Yes | Yes | No; plain streamed text |

Antigravity's project-local `/root/.gemini/antigravity-cli` state survives
stop/start and container replacement. After signing in with `agy` in the
project Terminal, choose **Refresh models** so the picker replaces any
signed-out fallback.

MiniMax's `/root/.minimax` model catalog and sessions also survive replacement,
separately from `/root/.codex`. Its key remains in the project secret store.

## Workspace tools

| Feature | How to use it | Limits or lifecycle |
| --- | --- | --- |
| Open in IDE | Choose **Open in IDE** in a project chat | code-server in `/workspace`; registered-user auth caveat |
| Installable apps | Use the browser's install action on Remote or `code.<host>` | Main app is network-first with a cached offline status page; IDE launcher keeps a live project list |
| Open Terminal | Choose **Open Terminal** | Resizable pane; hiding it in the same chat preserves the PTY; socket loss or page/chat change ends it |
| Open History | Choose **History** | Git repositories only |
| Open Files | Choose **Files** | Lazy workspace tree |
| Open Browser | Choose **Open Browser** | Preview or Agent Browser |
| Refresh Files | Use the drawer refresh control | Reloads root |
| Expand folder | Select folder row | Lazy-loads children |
| Open file | Select a file or search result | Supported media opens in-app; other non-archives open in IDE; unsupported media/archives download |
| Search filenames | Type at least two characters | 300 results; 200,000 visited-entry cap |
| Download file | Hover a file and choose download | Direct file response |
| Download folder | Hover a folder and choose download | ZIP up to 1 GiB; two concurrent |
| Inline media link | Open a supported workspace link from chat | Images, audio, video, PDF allowlist and full-screen viewer |
| IDE file link | Open a validated workspace path from chat or Files | Path-contained redirect with optional exact `:line[:column]` |

The **History** button is hidden until Remote discovers at least one repository.
It checks again after every completed run. The diff view groups files into
collapsible cards with line-number gutters, hunk headers, change counts, and
new/deleted/binary badges; unparseable patches fall back to raw text.

## Scheduled tasks

| Feature | How to use it | Important behavior |
| --- | --- | --- |
| Create | Select **Scheduled Tasks** skill and explicitly ask the agent | Agent-created tasks start paused |
| Arm | Open **Schedules** and select **Arm** | Human review is required before the first automatic run |
| One-time timing | Ask for an exact time with timezone | One successful occurrence |
| Recurring timing | Ask for five-field cron plus IANA timezone | Default minimum interval is 5 minutes |
| Edit | Open the task editor | Name, prompt, time/cron, timezone, max runs |
| Pause or resume | Use the task's primary control | Only paused tasks can resume |
| Run now | Use **Run now** | Does not move the normal deadline |
| Delete | Use the trash control and confirm | Removes definition and run history |
| Observe | Read next/last run, result, count, owner, and error | Runs appear as ordinary turns in the chat transcript |
| Complete standing task | Agent calls the scoped completion command during a scheduled run | Stops future runs and retains history |

Schedules exist only for project chats. Members see and manage their own tasks;
admins can see and manage all tasks. Defaults are 20 standing tasks per
project and two concurrent scheduled runs server-wide. Busy occurrences
coalesce into one follow-up under the default overlap policy.

## App preview and inspection

| Feature | How to use it | Boundary |
| --- | --- | --- |
| Discover apps | Open Browser or choose refresh | Non-loopback listeners, ports 1024–65535 |
| Select app | Use the process/port picker | One preview target at a time |
| Remember mentioned URL | Agent mentions a matching project preview URL | UI prefers the recent matching URL |
| Resize drawer | Drag the divider | Persists in browser local storage |
| Reload | Use the reload control | Reloads selected frame |
| Open externally | Use the external-link control | New authenticated tab |
| Inspect element | Toggle crosshair, then click the element | Same-origin inspector wrapper |
| Insert element context | Happens after selection | Selector, text, HTML, bounds, styles, parents |
| Preview authentication | Sign in to Remote | Admin or project member |
| Preview hostname | `slug--port.dev.<host>` | On-demand TLS and known-project check |

## Agent Browser

| Feature | How to use it | Boundary |
| --- | --- | --- |
| Start human browser view | Toggle the key control in Browser | Starts project Chromium/noVNC as needed |
| Share login with agent | Sign in visually, then select `browser` skill | Claude, Codex, and MiniMax share the same profile/window |
| Human intervention | Type or click in the live pane | Same session as agent |
| Reload view | Use reload while ready | Reloads noVNC iframe |
| Close drawer | Close Browser | Stops only the human view |
| Stop complete stack | Use the square stop control | Stops browser core and view; keeps profile |
| Persist site sessions | Reopen the project browser | Profile lives in workspace |
| Automatic reaping | Leave view and agent inactive | Stops after roughly 20 minutes |

There is one fixed 1366×768 browser session per project, unrestricted network egress, and no per-task browser isolation.

## Git history

| Feature | How to use it | Behavior |
| --- | --- | --- |
| Discover repositories | Open **History** | Workspace root through depth 6 with exclusions |
| Select repository | Use repository picker | Shows path and dirty/clean state |
| List commits | Select repository | UI asks for 100; backend caps at 200 |
| View commit | Select a commit | Shows subject, author, date, SHA |
| View diff | Select a commit | Structured per-file view; truncates at 768 KiB |
| Refresh | Use refresh control | Re-discovers repository state |
| Switch commit | Select a commit and choose **Switch** on a clean tree | Detached HEAD checkout |
| Safety checkpoint backend | Checkout API accepts a checkpoint message | Backend stages all, commits, then switches; current drawer does not render the required dirty-tree form |

## Project settings

| Tab | Features |
| --- | --- |
| Info | Container/OS/resources/disks/network/mounts/limits/agent versions/auth-bundle state, refresh, network repair |
| Settings | Effective and override limits; start, stop, force restart, admin-only delete |
| Secrets | Add, reveal, hide, edit, and delete environment values, including multiline values |
| Sharing | Add or remove registered project members |

Resource defaults are 6 CPUs, 4 GiB memory, and 2,000 processes. Admins alone may change limits or delete a project.

## Global settings

| Tab | Features |
| --- | --- |
| Appearance | System, Dark, Light |
| Notifications | Per-device Web Push permission, subscription, status, and test notification |
| Agents | Module-driven admin cards: managed sign-in/status/refresh, external instructions, or no-auth status |
| Users | Google OAuth configuration; add/remove users; member/admin roles |
| Updates | Current release, update checks, and administrator-triggered update flow |
| Info | Host CPU, memory, disks, network, OS/runtime, process, paths, role |

## Persistence reference

| State | Persists across page reload | Persists across container replacement |
| --- | ---: | ---: |
| Project metadata and membership | Yes | Yes |
| Chat metadata and event log | Yes | Yes |
| `/workspace` | Yes | Yes |
| Provider homes | Yes | Yes |
| Project secrets | Yes | Yes |
| Agent Browser profile | Yes | Yes |
| Scheduled-task definitions and run state | Yes | Yes |
| Antigravity sign-in and conversation state | Yes | Yes |
| Container root filesystem additions | Yes until replacement | No |
| Active run control and event streaming | No; an `lxc exec` child can remain alive but orphaned after backend restart | No reattachment |
| Active Terminal PTY | No | No |
| Composer draft | Yes, in the same browser tab session | Not applicable |
| Prompt queue | Yes, in the same browser tab session | Not applicable |
| Active chat and open drawers | No | Not applicable |
| Sidebar width/collapse, Browser drawer width, and Terminal pane width | Yes, in the same browser | Yes, in the same browser |
| Project-group collapsed state | No | Recomputed from unread state rather than stored |

## Features Remote does not currently provide

- anonymous/public preview sharing;
- read-only project membership;
- per-user provider accounts or quotas;
- enforced approval prompts for agent tools;
- durable server-side prompt queues;
- run reattachment after backend restart;
- terminal reconnect;
- the visible Git dirty-tree checkpoint form, despite backend support;
- project IDE membership enforcement;
- built-in backup/restore, audit log, metrics endpoint, or high availability;
- content search in Files;
- offline access to the live app shell, chats, project data, or agent controls;
- current application voice dictation;
- implemented `@`-mention or slash-command composer menus.
- a direct “create schedule” form in the current UI; schedule creation starts
  through the Scheduled Tasks skill, then the drawer manages the definition.

Read [Known limitations](../known-limitations.md) and the [Threat model](../threat-model.md) before using Remote with mutually untrusted users or high-value credentials.

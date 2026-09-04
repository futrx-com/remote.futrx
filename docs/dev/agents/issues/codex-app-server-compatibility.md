# Codex App Server compatibility gaps

| Field | Value |
| --- | --- |
| Status | Open |
| Audit baseline | `agents/improvements` at `37f6dde4e5a829843ace7fadf534ea529f3bf89a` |
| Codex baseline | `@openai/codex` `0.149.1` |
| Scope | Remote adapter, transport, state, and UI only |

Remote uses the official `codex app-server` JSON-RPC interface, which is the
correct boundary for a graphical Codex client. Basic turns work, but Remote is
not yet a complete or faithful Codex UI: it answers some server requests on the
user's behalf, flattens or drops protocol data, and substitutes process-level
behavior for native turn lifecycle operations.

The governing constraint for every item in this document is:

> Codex and its App Server are the provider-owned harness. Do not patch,
> replace, or reproduce that harness. Remote's responsibility is to transport,
> retain, present, and answer the protocol as a UI client.

The protocol reference is the
[official Codex App Server documentation](https://developers.openai.com/codex/app-server/).
The exact compatibility baseline is the version pinned in
[`versions.env`](../../../../backend/internal/agent/provisioning/versions.env).
At the time of the audit, the pinned `0.149.1` schema was also compared with
the then-current `0.152.0` schema to identify forward-compatibility risks.

## Verdict

Remote currently supports the Codex happy path:

- launching the official App Server and completing its run-time
  `initialize`/`initialized` handshake;
- starting, resuming, and forking native Codex threads;
- starting a text turn with model, reasoning-effort, and service-tier choices;
- streaming assistant and reasoning text;
- showing snapshots for several common tool calls;
- persisting thread identity and final token usage; and
- discovering the live model catalog, reasoning levels, service tiers, and
  the presence of Plan mode.

That is enough for ordinary text chat, but not for full compatibility. Full
compatibility here means that every protocol interaction supported by the
pinned Codex version is either rendered natively or shown through an explicit,
lossless fallback; server requests remain pending until the user answers them;
and unknown additions do not silently disappear or cause Remote to make a
policy decision.

## Issue summary

| ID | Priority | Gap | User-visible result |
| --- | --- | --- | --- |
| CUI-001 | P0 | User-input RPCs are answered before the user responds | Answers do not reach the turn that asked the question |
| CUI-002 | P0 | Remote decides approvals and elicitation | Mutations can be accepted and prompts cancelled without UI consent |
| CUI-003 | P1 | The event bridge is a lossy whitelist | New and rich Codex activity silently disappears |
| CUI-004 | P1 | Errors and retries are decoded incorrectly | Auth, network, and retry failures can be invisible |
| CUI-005 | P1 | Cancellation and status do not use native turn semantics | Interrupted turns can look successful and waiting states are hidden |
| CUI-006 | P1 | Plans are flattened into assistant prose | Step state and plan identity are lost |
| CUI-007 | P1 | Tool progress and rich results are discarded | The UI shows partial snapshots instead of the live operation |
| CUI-008 | P1 | Native multimodal inputs and outputs are not represented | Files, images, audio, skills, and generated media degrade to text or vanish |
| CUI-009 | P2 | Continuity is reconstructed outside Codex in several flows | Recovery loses native history and steering becomes a later turn |
| CUI-010 | P2 | Capability and collaboration metadata are only partly honored | Controls can misrepresent the server's supported configuration |
| CUI-011 | P2 | Authentication exposes only a Remote-owned subset | Some Codex account state and project-local auth behavior are not visible |
| CUI-012 | P2 | Protocol lifecycle assumptions are brittle | A valid server response or newer schema can break an otherwise valid turn |

## CUI-001: user-input RPCs are detached from the pending turn

Codex server requests such as `item/tool/requestUserInput` carry a JSON-RPC
request ID. Remote must keep that request pending and answer the same ID after
the user responds. Instead,
[`app_server_requests.go`](../../../../backend/internal/integration/agents/codex/app_server_requests.go)
emits an `AskUserQuestion` tool card and immediately sends an answer containing
empty arrays for every question.

The visible card is therefore no longer connected to the Codex request. Its UI
handler in
[`useAskUserQuestion.ts`](../../../../frontend/src/ui/chat/tool-calls/ask-user-question/useAskUserQuestion.ts)
formats the selection as prose and submits it through the normal composer. The
normal prompt path in
[`useChat.ts`](../../../../frontend/src/state/hooks/chat/useChat.ts) rejects a
new prompt while the current run is streaming. If submitted later, it becomes
a separate turn rather than the answer Codex requested. The card can still be
persisted as answered, which makes the displayed state misleading.

The request decoder also retains only a subset of the pinned question schema.
Fields including `isOther`, `isSecret`, `isBlocking`, `autoResolutionMs`, and
nullable options are not represented. In particular, a secret answer must
never be copied into visible chat prose or logs.

Required UI-only behavior:

- preserve JSON-RPC request ID, thread ID, turn ID, item ID, question IDs, and
  the full question schema in pending interaction state;
- add an explicit WebSocket response message instead of calling `sendPrompt`;
- return the user's values on the original JSON-RPC request;
- keep blocking state visible while the request is pending;
- support cancellation, timeout/auto-resolution, and server-side resolution;
  and
- redact secret values from chat history, logs, replay, and diagnostics.

## CUI-002: approvals and elicitation are policy-decided by Remote

[`app_server_requests.go`](../../../../backend/internal/integration/agents/codex/app_server_requests.go)
currently accepts command and file approvals in Default mode, declines them in
Plan mode, and always cancels MCP elicitation. Older approval methods are also
answered locally. Unknown request methods receive JSON-RPC `-32601`, including
new permission and dynamic-tool request families that the UI does not yet
implement.

The adapter also sends `approvalPolicy: "never"` and unrestricted sandbox
settings when building both thread and turn requests in
[`app_server_protocol.go`](../../../../backend/internal/integration/agents/codex/app_server_protocol.go).
Plan mode is a collaboration preset, not an authorization boundary, so it must
not silently stand in for a user approval policy.

Frontend models mention permission messages, but the chat WebSocket in
[`chat_socket.go`](../../../../backend/internal/transport/ws/chat_socket.go)
accepts only prompt and cancel operations, and
[`chatStream.ts`](../../../../frontend/src/api/chat/chatStream.ts) has no
approval-response operation.

Required UI-only behavior:

- make the selected approval and sandbox policy explicit in the UI;
- transport every approval, permission, and elicitation request to the client;
- keep the upstream request pending and answer its exact JSON-RPC ID;
- show the effect and scope of each decision, including session-scoped choices;
- distinguish user denial, server cancellation, timeout, and unsupported
  request types; and
- never approve, decline, or cancel merely because of a Remote run mode.

## CUI-003: the event bridge is a lossy whitelist

The neutral event model in
[`backend/internal/agent/model.go`](../../../../backend/internal/agent/model.go)
supports session, text, reasoning, basic tool, usage, completion, and error
events. The Codex parser in
[`app_server_events.go`](../../../../backend/internal/integration/agents/codex/app_server_events.go)
recognizes only a subset of notification methods and item types. Raw envelopes
are temporarily retained on `agent.Event`, then discarded by
[`agent_events.go`](../../../../backend/internal/service/prompt/agent_events.go)
when events become persisted chat events. The frontend message model in
[`chatMessage.ts`](../../../../frontend/src/models/chatMessage.ts) consequently
has no way to render the omitted structures.

Pinned item families that can disappear include image generation and viewing,
sub-agent activity, sleep, review-mode transitions, context compaction, and
completed reasoning. Omitted notifications include thread status, structured
plan updates, diffs, command and file-output deltas, terminal interaction, MCP
progress, warnings, model rerouting and verification, safety buffering, hooks,
and request resolution.

This makes every new Codex feature a coordinated parser/model/UI release and
causes unknown protocol additions to fail invisibly.

Required UI-only behavior:

- carry a versioned Codex envelope, including native IDs and raw payload,
  through persistence and transport;
- implement typed projections for features that have dedicated UI;
- render an explicit generic item/notification fallback for valid unknown
  methods and item types;
- preserve ordering and native lifecycle state; and
- reserve normalization for genuinely provider-neutral UI concerns rather
  than erasing provider semantics.

## CUI-004: errors and retry state are decoded incorrectly

The pinned App Server error notification contains a nested `error` object and
retry metadata such as `willRetry`. Remote's `appServerErrorParams` in
[`app_server_protocol.go`](../../../../backend/internal/integration/agents/codex/app_server_protocol.go)
expects a top-level `message`. The `error` branch in
[`app_server_events.go`](../../../../backend/internal/integration/agents/codex/app_server_events.go)
therefore drops normal Codex error notifications.

This is not cosmetic: authentication, quota, network, model, and transient
retry failures may leave the UI apparently streaming or complete with no
explanation. Retryable errors must not be projected as indistinguishable
terminal failures.

Required UI-only behavior:

- decode the complete pinned error shape and retain its structured metadata;
- visibly distinguish retrying, recovered, and terminal errors;
- associate an error with its thread, turn, item, or request where supplied;
- keep safe provider messages intact without exposing secrets; and
- provide a generic fallback for future error fields.

## CUI-005: cancellation and live status bypass native turn semantics

Every terminal status other than literal `failed` is currently projected as a
successful completion in
[`app_server_events.go`](../../../../backend/internal/integration/agents/codex/app_server_events.go).
That includes `interrupted` and any future non-success terminal state.

Cancel flows through the generic run hub in
[`hub.go`](../../../../backend/internal/service/runhub/hub.go) and cancels the
process context. Because Codex is launched with `exec.CommandContext` in
[`command.go`](../../../../backend/internal/integration/agents/codex/command.go),
Remote kills the App Server rather than sending native `turn/interrupt` and
waiting for its terminal notification. Cancellation is then treated as a clean
return by
[`app_server_run.go`](../../../../backend/internal/integration/agents/codex/app_server_run.go).

Codex states such as `waitingOnApproval` and `waitingOnUserInput` are ignored.
The visible running state is Remote's process-local run lock rather than the
authoritative thread/turn status.

Required UI-only behavior:

- retain the turn ID returned by `turn/start`;
- send `turn/interrupt` for user cancellation and await the corresponding
  terminal state;
- map every known turn status explicitly, never by "not failed means success";
- render waiting, retrying, interrupted, failed, and completed states; and
- use process termination only as transport-failure cleanup, with that failure
  shown distinctly from a native interruption.

## CUI-006: plans are flattened into ordinary assistant prose

Both `item/plan/delta` and agent-message deltas become
`assistant.delta` in
[`app_server_events.go`](../../../../backend/internal/integration/agents/codex/app_server_events.go).
Completed plan items follow the same path, while structured
`turn/plan/updated` notifications and step statuses are ignored. The current
App Server run test in
[`app_server_run_test.go`](../../../../backend/internal/integration/agents/codex/app_server_run_test.go)
explicitly asserts the flattened assistant-text behavior.

The user can read plan prose, but cannot see stable steps, pending/in-progress/
completed state, revisions, or the distinction between a plan and the final
answer.

Required UI-only behavior:

- add a plan message part keyed by native plan/turn IDs;
- consume structured plan updates as authoritative snapshots;
- preserve step order and status across replay and reconnect; and
- keep prose fallback only for servers that do not provide structured plans.

## CUI-007: tool progress and rich results are discarded

Command starts retain only the command string. CWD, parsed actions, source,
process ID, status, and other native fields are dropped. Live command/file
output deltas, terminal interaction, diffs, and MCP progress are ignored, so a
tool generally appears as a spinner followed by one final snapshot. The Bash
renderer in
[`BashCall.tsx`](../../../../frontend/src/ui/chat/tool-calls/renderers/BashCall.tsx)
also truncates the displayed final output to 6,000 characters.

The pinned `dynamicToolCall` completion shape uses `contentItems` and
`success`, while `appServerItem` currently expects `result` and `error`.
[`appServerToolOutput`](../../../../backend/internal/integration/agents/codex/app_server_events.go)
therefore commonly collapses rich text, image, or audio output to a status
string. The generic renderer in
[`GenericCall.tsx`](../../../../frontend/src/ui/chat/tool-calls/renderers/GenericCall.tsx)
supports only JSON/text presentation.

Required UI-only behavior:

- preserve the full start, delta, interaction, and completion lifecycle;
- represent command metadata, incremental output, file diffs, and MCP progress;
- decode the pinned completion schema for each tool family;
- render typed text/image/audio result items and an explicit unknown fallback;
- keep full persisted output available even when the initial view is collapsed
  or visually truncated; and
- correlate all updates by native item ID.

## CUI-008: native multimodal inputs and outputs are not represented

Uploads are converted to an `Attached files:` prose fragment in
[`chatAttachmentService.ts`](../../../../frontend/src/services/chat/chatAttachmentService.ts).
Every turn built by
[`app_server_protocol.go`](../../../../backend/internal/integration/agents/codex/app_server_protocol.go)
contains exactly one text input even though the pinned protocol supports native
image, local-image, audio, skill, and mention inputs. Selected skills are also
injected into prompt text by
[`prompt/service.go`](../../../../backend/internal/service/prompt/service.go)
instead of being sent as Codex skill inputs.

On output, image-view and image-generation items are ignored, and the frontend
assistant model has no media part. This loses both semantics and display data.

Required UI-only behavior:

- build the appropriate native `UserInput` variant for every attachment and
  selected skill the server supports;
- validate model input modalities before submission;
- keep text fallback deliberate and visible rather than implicit;
- persist and render generated/viewed images, audio, and future content items;
  and
- retain content metadata and native IDs without embedding secrets or binary
  payloads in prompt prose.

## CUI-009: continuity and steering are reconstructed outside Codex

Native `thread/start`, `thread/resume`, and `thread/fork` are correctly used,
and new thread IDs and resolved models are persisted. Chat forks also defer to
Codex's native fork operation when the descriptor permits it. These are strong
parts of the current integration.

However, rewind clears the Codex session ID in
[`filechat/store.go`](../../../../backend/internal/stores/filechat/store.go).
The next turn reconstructs context using a bounded visible prose transcript in
[`prompt/service.go`](../../../../backend/internal/service/prompt/service.go).
Missing-thread recovery does the same. The reconstructed transcript omits tool
state, reasoning, native items, and older context.

Input composed during a run is queued as another Remote turn. Native
`turn/steer` is not used, so steering intent cannot affect the active turn.

Required UI-only behavior:

- prefer native thread operations whenever Codex exposes them;
- expose the lossy nature of transcript recovery rather than presenting it as
  full continuity;
- preserve native history/item references needed by supported rollback flows;
- offer `turn/steer` for active-turn input when supported, while keeping a
  clearly distinct "next turn" queue; and
- distinguish missing-thread recovery from ordinary resume in persisted UI
  state.

## CUI-010: capabilities and collaboration settings are incomplete

The adapter initializes App Server, pages through `model/list`, and queries
`collaborationMode/list` in
[`capability_app_server.go`](../../../../backend/internal/integration/agents/codex/capability_app_server.go).
[`capabilities.go`](../../../../backend/internal/integration/agents/codex/capabilities.go)
then exposes live models, supported reasoning efforts, service tiers, and Plan
availability. Composer options are driven from that catalog. This is the most
complete part of the integration.

The projection still discards model input modalities, personality support,
multi-agent metadata, and upgrade metadata. Collaboration-mode discovery keeps
only the name and mode, dropping the server-provided model, reasoning preset,
and other settings. Turn construction then recreates Plan mode locally and
forces medium reasoning when no effort was chosen.

The separate capability probe also sends `model/list` and
`collaborationMode/list` after the `initialize` response without first sending
the required `initialized` notification. The pinned CLI tolerates this today,
but it is not a complete protocol handshake. Capability warnings and fallback
status are returned by the backend but are generally not rendered in the
composer.

Required UI-only behavior:

- complete the same `initialize`/`initialized` lifecycle used by normal runs;
- retain the full model and collaboration-mode records from the server;
- derive defaults and Plan settings from those records rather than recreating
  them in the adapter;
- use input modalities and feature metadata to enable or disable UI controls;
- surface discovery warnings, fallback source, and refresh state; and
- tolerate additive schema fields without losing them.

## CUI-011: authentication exposes a Remote-owned subset

Remote provides a functional host-managed `codex login --device-auth` flow in
[`auth.go`](../../../../backend/internal/integration/agents/codex/auth.go) and
intentionally rejects host API-key authentication so runs use ChatGPT
subscription limits. It strips `OPENAI_API_KEY` from the run environment in
[`command.go`](../../../../backend/internal/integration/agents/codex/command.go).

This is narrower than the complete App Server account surface. Account/login
state and related provider notifications are not carried through the live
Codex client protocol. In project runs, credential seeding does not overwrite a
newer project-local auth record. A project-local API-key record can therefore
drive a run; the later credential pull detects it only after completion, and
that sync failure is logged without failing the completed run.

Required UI-only behavior:

- clearly document and display the intentionally supported authentication
  modes;
- surface authoritative account state, entitlement, quota, and login changes
  that App Server provides;
- inspect effective project auth before launch under the existing product
  policy; and
- show a policy mismatch before a billed or otherwise unintended run begins.

Supporting an additional auth mode is a product decision, not a requirement to
modify the Codex harness.

## CUI-012: protocol lifecycle and schema assumptions are brittle

Normal runs correctly send `initialized`, but thread response handling in
[`app_server_run.go`](../../../../backend/internal/integration/agents/codex/app_server_run.go)
requires both `thread.id` and a top-level `model`. The documented thread
response contract is thread-centric; treating the extra top-level model as
mandatory can reject a valid response. Resolved model data should be read from
the authoritative response location for the negotiated schema, with the
requested/discovered model as a safe UI fallback.

The adapter also assumes numeric response IDs from a small fixed set and uses a
closed parser switch. The legacy `codex exec --json` parser in
[`parser.go`](../../../../backend/internal/integration/agents/codex/parser.go)
is still tested but is not the production App Server path, which can give a
misleading impression of coverage.

Required UI-only behavior:

- generate or validate client types against the exact pinned App Server schema;
- add fixtures for every pinned request, response, notification, item, status,
  and content variant;
- test the production App Server path rather than using legacy JSONL coverage
  as a proxy;
- allow additive fields and server-generated string or numeric request IDs;
- negotiate or fail visibly on genuinely incompatible protocol changes; and
- compare schemas before every Codex pin upgrade.

## Target architecture

The fix belongs entirely on Remote's side of the App Server boundary:

1. A Codex protocol layer owns JSON-RPC framing and the generated or
   schema-validated native types. It retains request IDs and never makes UI
   policy decisions.
2. A persisted interaction layer records thread, turn, item, request, status,
   and resolution state without flattening provider data.
3. The WebSocket transports native-correlated notifications and explicit user
   responses in both directions.
4. Frontend projectors create typed views for known items and a safe generic
   view for unknown ones.
5. The UI is the decision point for questions, approvals, permissions,
   elicitation, interruption, and steering.
6. The adapter returns the UI's response to the original Codex JSON-RPC request
   and treats App Server's completion/resolution notification as authoritative.

Provider-neutral events may remain for shared chat features, but they cannot be
the only persisted representation when doing so destroys Codex semantics.

## Implementation order

1. **Pending interactions:** introduce native IDs and two-way response
   transport; fix questions, approvals, permissions, and elicitation first.
2. **Lifecycle correctness:** decode errors/status, add native interrupt, and
   render waiting/retrying/interrupted state.
3. **Lossless persistence:** retain native envelopes and add an unknown-event
   fallback before expanding the typed renderer set.
4. **Structured UI:** add plans, live tool progress, diffs, collaboration
   activity, and rich content items.
5. **Native input:** send attachments, media, mentions, and skills using their
   supported `UserInput` variants; add steering.
6. **Discovery and upgrades:** consume complete capability/mode metadata and
   enforce schema-diff checks for pin changes.

## Acceptance criteria

- A Codex question remains pending until the user responds, and the response is
  sent once on the same JSON-RPC ID without creating a new turn.
- Secret questions never persist or render their answer as ordinary chat text.
- No approval, permission, or elicitation request is silently accepted,
  declined, denied, or cancelled by adapter mode logic.
- Cancel sends native `turn/interrupt`; the UI distinguishes interrupted from
  completed and from transport termination.
- Nested errors and `willRetry` state are visible and correctly correlated.
- Every item and notification in the pinned schema has either a typed renderer
  or an explicit generic fallback; none disappears silently.
- Structured plans retain stable steps and status across reconnect and replay.
- Tool deltas, diffs, progress, and rich result content survive persistence and
  are renderable without relying on a truncated final string.
- Supported attachments and skills are native inputs, and supported generated
  media is a native output part.
- Capability discovery sends `initialized` and the UI honors server-provided
  modalities, defaults, mode settings, and warnings.
- Thread responses are decoded according to the pinned schema without requiring
  undocumented convenience fields.
- Contract tests cover the generated `0.149.1` surface and fail clearly on an
  incompatible future pin.
- All implementation changes remain in Remote's adapter, services, transport,
  persistence, tests, and frontend. No Codex harness modification is required
  or permitted.

## Primary source locations

- [`backend/internal/integration/agents/codex/`](../../../../backend/internal/integration/agents/codex)
- [`backend/internal/agent/model.go`](../../../../backend/internal/agent/model.go)
- [`backend/internal/service/prompt/`](../../../../backend/internal/service/prompt)
- [`backend/internal/service/runhub/hub.go`](../../../../backend/internal/service/runhub/hub.go)
- [`backend/internal/transport/ws/chat_socket.go`](../../../../backend/internal/transport/ws/chat_socket.go)
- [`frontend/src/models/chat.ts`](../../../../frontend/src/models/chat.ts)
- [`frontend/src/models/chatMessage.ts`](../../../../frontend/src/models/chatMessage.ts)
- [`frontend/src/api/chat/chatStream.ts`](../../../../frontend/src/api/chat/chatStream.ts)
- [`frontend/src/state/hooks/chat/`](../../../../frontend/src/state/hooks/chat)
- [`frontend/src/ui/chat/tool-calls/`](../../../../frontend/src/ui/chat/tool-calls)

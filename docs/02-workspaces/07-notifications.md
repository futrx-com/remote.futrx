# Notifications

Agent runs are long. This page covers the feature that lets an operator walk
away from one and still learn, on their phone, when it finished, failed, or
stopped to ask a question — plus how a scheduled task reported back.

Notifications are a **global, admin-owned** setting: one configuration for the
whole server, delivered to at most two destinations (a Telegram chat and one
generic webhook). There is no per-user or per-project routing yet.

## What triggers a notification

| Event | Fires when | `status` values |
| --- | --- | --- |
| `runFinished` | An interactive agent turn completed without error | `finished` |
| `runFailed` | An interactive agent turn errored or was cancelled | `failed`, `cancelled` |
| `needsAttention` | The agent called a tool that hands control back to a human | `waiting` |
| `scheduledRun` | A scheduled task run settled | `succeeded`, `failed` |

Two details worth knowing:

- **A scheduled run never produces both.** Runs injected by the scheduler are
  skipped by the interactive `runFinished`/`runFailed` path so a nightly task
  pings once, not twice.
- **`needsAttention` is tool driven.** Agents launch with permission prompts
  disabled, so there is no interactive approval event to hook. What remains is
  the set of tools that exist precisely to ask the human: `AskUserQuestion`,
  `ExitPlanMode`, `RequestPermission`, and their snake- and kebab-case
  spellings (see `attentionTools` in
  [`backend/internal/service/notify/events.go`](../../backend/internal/service/notify/events.go)).
  Each such call is its own ping — an agent that asks twice notifies twice.

Every notification carries a deep link back to the chat:
`https://<your-host>/?chat=<chatId>`. The SPA reads that parameter once on
boot, selects the chat, and strips it from the address bar.

## Delivery guarantees (or the lack of them)

Delivery is deliberately best effort, because a notification must never be able
to break a run:

- Events go onto a **bounded in-memory queue** (256 entries) drained by a single
  worker goroutine. A full queue drops the newest event and logs it.
- Each sink gets **three attempts** with a 1s then 3s backoff, and every outbound
  HTTP call has a **10 second timeout**.
- **One "finished" per run.** Events carry an internal dedupe key
  (`run:<chatId>:<runId>`, `schedule:<taskId>:<lastRunAt>`); the last 512 keys
  are remembered.
- The queue is **not durable**. Events pending at a restart are lost.

## Enabling notifications

Settings → **Notifications** (admin only). Fill in at least one destination,
tick the events you want, turn **Send notifications** on, save, then use
**Send test** — it reports per-sink success or the exact error, which is the
fastest way to find a wrong token or a chat the bot was never added to.

Configuration lives at `DATA_DIR/notifications.json`, mode `0600` because it
holds the bot token and the webhook secret. There are no environment variables
to set; the deep link reuses `BASE_URL`.

Secrets are **write-only over the API**. `GET /api/admin/notifications` returns
`••••` plus the last four characters, never the value. Saving with a blank
secret field keeps whatever is stored, which is why the form can show a mask
instead of the real credential; the **Remove stored …** buttons clear one.

## Telegram

### 1. Create a bot

1. Open Telegram and message [@BotFather](https://t.me/BotFather).
2. Send `/newbot`, pick a display name and a username ending in `bot`.
3. BotFather replies with a token shaped like
   `123456789:AAHk9y...`. That is the **bot token**.

### 2. Find the chat ID

A bot cannot message you first, so you must open the conversation:

- **Direct messages:** send your new bot any message (`/start` works).
- **Group:** add the bot to the group, then send a message mentioning it.
  For groups, also send `/setprivacy` → `Disable` to BotFather if the bot does
  not appear to see messages.

Then read the chat ID:

```bash
curl -s "https://api.telegram.org/bot<TOKEN>/getUpdates" \
  | grep -o '"chat":{"id":[-0-9]*'
```

Personal chats have positive IDs (`123456789`); groups and supergroups have
negative ones (`-1001234567890`). Paste it into the **Chat ID** field.

### Message format

Messages are sent with `parse_mode: HTML`. Everything interpolated — project
name, chat title, agent output — is HTML-escaped first, so agent output cannot
inject markup into your Telegram client. Summaries are trimmed to 900
characters and the whole message to 3500.

## Generic webhook

Remote sends `POST <your URL>` with `Content-Type: application/json` and this
body:

```json
{
  "event": "runFinished",
  "projectId": "9f2a1c04",
  "projectSlug": "acme-api",
  "projectName": "Acme API",
  "chatId": "abc123",
  "chatTitle": "Fix the login redirect",
  "provider": "claude",
  "status": "finished",
  "summary": "Updated the session cookie domain and added a regression test.",
  "url": "https://remote.example.com/?chat=abc123",
  "at": 1700000000000
}
```

| Field | Type | Notes |
| --- | --- | --- |
| `event` | string | `runFinished`, `runFailed`, `needsAttention`, `scheduledRun`, or `test` |
| `projectId`, `projectSlug`, `projectName` | string | Omitted for loose (project-less) chats |
| `chatId`, `chatTitle` | string | `chatTitle` omitted if the chat has no title yet |
| `provider` | string | `claude`, `codex`, `kimi`, or `antigravity` |
| `status` | string | See the trigger table above |
| `summary` | string | Agent output or failure reason, trimmed to 600 characters |
| `url` | string | Deep link to the chat |
| `at` | number | Unix milliseconds |

Any `2xx` response counts as delivered. Anything else is retried up to the
three-attempt limit.

### Verifying the signature

When a shared secret is configured, each request carries

```
X-Remote-Signature: sha256=<hex hmac-sha256 of the raw body>
```

Compute the HMAC over the **exact bytes received**, before any JSON parsing or
re-serialization, and compare in constant time.

```js
// Node.js / Express — note express.raw(), not express.json()
import crypto from "node:crypto";

app.post("/remote", express.raw({ type: "application/json" }), (req, res) => {
  const expected =
    "sha256=" +
    crypto.createHmac("sha256", process.env.REMOTE_WEBHOOK_SECRET).update(req.body).digest("hex");
  const received = req.get("X-Remote-Signature") ?? "";

  const a = Buffer.from(expected);
  const b = Buffer.from(received);
  if (a.length !== b.length || !crypto.timingSafeEqual(a, b)) {
    return res.sendStatus(401);
  }

  const event = JSON.parse(req.body.toString("utf8"));
  console.log(event.event, event.chatTitle, event.url);
  res.sendStatus(204);
});
```

```python
# Python / Flask
import hmac, hashlib, os
from flask import request, abort

@app.post("/remote")
def remote():
    body = request.get_data()  # raw bytes
    expected = "sha256=" + hmac.new(
        os.environ["REMOTE_WEBHOOK_SECRET"].encode(), body, hashlib.sha256
    ).hexdigest()
    if not hmac.compare_digest(expected, request.headers.get("X-Remote-Signature", "")):
        abort(401)
    event = request.get_json()
    return "", 204
```

## API reference

All three routes are admin-only; a registered non-admin gets `403`.

| Method | Path | Body | Response |
| --- | --- | --- | --- |
| `GET` | `/api/admin/notifications` | — | Masked configuration |
| `PUT` | `/api/admin/notifications` | Update payload (below) | Masked configuration |
| `POST` | `/api/admin/notifications/test` | — | `{"results":[{"sink","configured","delivered","error"}]}` |

```json
{
  "enabled": true,
  "telegram": { "botToken": "", "clearBotToken": false, "chatId": "-1001234567890" },
  "webhook": { "url": "https://hooks.example.com/remote", "secret": "", "clearSecret": false },
  "events": { "runFinished": true, "runFailed": true, "needsAttention": true, "scheduledRun": true }
}
```

A blank `botToken` or `secret` keeps the stored value; the matching `clear*`
flag removes it. `PUT` returns `400` when the configuration cannot work —
enabling with no destination, a bot token without a chat ID, a webhook secret
without a URL, or a webhook URL that is not absolute `http(s)`.

Test events ignore both the master switch and the event toggles, so an operator
can always debug a sink.

## Security notes

- The bot token and webhook secret sit in `notifications.json` **in plaintext**,
  mode `0600`, exactly like the other secrets under `DATA_DIR` (see the
  [threat model](../threat-model.md)). Anyone with root on the host can read
  them.
- Telegram errors are redacted before they reach the log or the admin UI, so a
  failing request never prints the bot token.
- The webhook URL is operator-supplied and **not restricted to public
  addresses** — an admin can point it at a host on the internal network. Treat
  it as an admin-level capability.
- Summaries contain agent output. Anyone who can read the Telegram chat or the
  webhook endpoint can read a slice of every agent run.

## Related

- [Chat and agents](04-chat-and-agents.md) — where run events come from
- [Scheduled tasks](06-scheduled-tasks.md) — the `scheduledRun` source
- [API and realtime](../03-platform/08-api-and-realtime.md) — the rest of the HTTP surface

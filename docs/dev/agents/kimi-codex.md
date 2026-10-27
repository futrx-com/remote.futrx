# Kimi through Codex

The `kimi-codex` module is an additional project-scoped provider. The original
`kimi` module remains on its native CLI and keeps its device authentication
and session format. No old chats or credentials are migrated.

Kimi documents both Claude Code and Codex integrations. Codex was chosen because
Kimi supports Responses natively and Remote already shares the app-server
runtime with MiniMax. A published K3 coding evaluation also favored Codex's
cost and speed; it is supporting evidence, not a guarantee for every workload.

- [Kimi's Codex integration](https://www.kimi.com/code/docs/en/third-party-tools/codex.html)
- [Kimi model configuration](https://www.kimi.com/code/docs/en/kimi-code/models)
- [FrontierHarness evaluation](https://frontierharness.org/)

The provider owns API-key validation, `/models` discovery, model metadata,
capabilities, the Codex catalog, and launch configuration. The existing
`codexharness` package owns streaming, tool events, interruption, and thread
resume/fork. The shared managed-key service owns private storage and named
accounts; the frontend uses its existing provider-driven authentication UI.

The endpoint is `https://api.kimi.ai/coding/v1`. Model discovery sends Remote's
own User-Agent; inference uses Codex's normal identity. Neither impersonates
Kimi's native client. API errors and redirects never expose response bodies or
keys. Failed discovery does not publish a guessed list of available models.

Kimi's `context_length`, image support, and `think_efforts` metadata feed both
the picker and Codex. Older catalogs get a conservative 256K context limit.
K3-256K is always capped at 262144. Unsupported or disabled reasoning falls
back to a supported enabled level, preventing silent model substitution.

The provisioning profile mounts `/root/.kimi-codex` independently of all other
provider homes. It provisions shared instructions, project skill links, the
existing pinned Codex binary, and browser assets using the existing lifecycle.
The catalog contains no credentials. Keys are injected per run, using the
chat's selected account; project secrets cannot override that key or home.

QA checks: save a Kimi Code key in Settings → Agents → Kimi (Codex), refresh
models, create a project chat with K3, ask it to edit and read a small file,
then check plan mode, Stop, resume, and fork. A real Kimi account with K3
entitlement is required to verify upstream inference; mocked transport tests
do not establish that entitlement.

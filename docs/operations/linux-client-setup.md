# Linux Client Setup

Linux servers can connect to the gateway without a desktop, CC Switch, or the
removed remote-control module.

## Quick Setup

1. Sign into the real gateway in a browser and open **API Keys**.
2. Select an active personal key, then choose **Install and Configure CLI** (the
   terminal icon).
3. Select **Codex CLI**, **Claude Code**, **Gemini CLI**, or **OpenCode**, and choose **Linux** explicitly even
   when the browser is running on macOS or Windows.
4. Copy the generated command and execute it on the server as the account that
   will run the client. Do not prefix the whole command with `sudo`.

The generated command has this shape (the example below is not executable):

```text
curl -fsSL 'https://YOUR_GATEWAY/install/ONE_TIME_CODE' | sh
```

Only execute scripts from your trusted gateway over HTTPS. The install code is
a bearer credential: keep it private. It is valid for 15 minutes and consumed on
download. Do not prefetch the URL to check it; doing so consumes the code. Create
a new command if it expires or installation needs to be retried.

The server needs `curl`, `python3`, and either the selected client binary or a
working Node.js/npm installation to install the missing client. Package-manager
permissions and client platform support still apply. The script does not install
system prerequisites or require the remote-control sidecar.

## Configuration Written

- Codex: updates the gateway provider in `~/.codex/config.toml`, selecting it as
  `model_provider`, using the gateway's `/v1` endpoint and Responses protocol.
  Existing unrelated settings are retained; `auth.json` is not overwritten.
- Claude Code: updates `ANTHROPIC_BASE_URL` and `ANTHROPIC_AUTH_TOKEN` under `env`
  in `~/.claude/settings.json`.
- Gemini CLI: writes `~/.gemini/.env` and updates `~/.gemini/settings.json`.
- OpenCode: uses the separate safe merge/sync flow described below.
- Claude/Codex/Gemini also write a mode-0600 `~/.aether/client.env`. The actual API key is stored
  locally in these configuration files; keeping it out of the copied install
  command does not make the installed key ephemeral.

After setup, run the selected client on that server and choose a public model
that the account and API key are allowed to use. This is client configuration,
not an SSH connection, VS Code pairing, or tunnel-node installation.

## OpenCode Setup and Sync

The OpenCode installer supports **Linux and macOS**. Native Windows/PowerShell
installation is not implemented: the UI disables that combination, and the API
rejects it instead of creating a nonfunctional Windows target. On Windows, use
WSL and select **Linux**. A missing OpenCode binary is installed with
`npm install -g opencode-ai`; an existing binary is reused.

The installer requests `GET /api/opencode/config` with `Authorization: Bearer`
using the selected API key. This endpoint must be deployed before installation;
it provides a key-free OpenCode config containing `provider.aether`, including
`options.baseURL` and the key's currently accessible models and full known
metadata. The local installer injects the key into the private client config.
For OpenCode token limits, the catalog resolves `context_limit` and `output_limit`
from each eligible provider-model association's `config`, inheriting absent fields
from the global model. It advertises the smallest effective values across the
providers accessible to the selected API key, rather than just the highest-priority
provider. Restricting that key to a provider therefore restricts the aggregation too.
Only positive, JavaScript-safe integer limits are published. Missing association
metadata, an invalid explicit override, an unknown effective limit, or output larger
than context causes the alias's `limit` to be omitted rather than guessed. Inactive
or unavailable associations do not contribute. Provider IDs, upstream names and
other association configuration are never included in the generated catalog.

These are configured limits, not measured upstream capacities. Operators still
need to establish the actual provider-specific values. This does not infer that an
upstream “high demand” error means context overflow, and it does not change Codex
installation, CC Switch imports, Codex model cards, or request admission limits.

The gateway must use HTTPS; plain HTTP is allowed only for loopback development.
URLs containing credentials, query parameters or fragments and HTTP redirects
are rejected. The key is never put into a request URL or command-line argument.

Files written:

- `~/.config/opencode/opencode.json` (0600), or
  `$XDG_CONFIG_HOME/opencode/opencode.json` when that absolute path is set during
  installation. It merges `provider.aether`: other providers and unrelated
  top-level preferences are preserved. Arca provider settings and options
  absent from the server response are retained, except credential headers:
  `Authorization`, `Proxy-Authorization`, `x-api-key`, `api-key`, `api_key`,
  `x-goog-api-key`, `x-auth-token` and `x-access-token` are removed case-insensitively
  from both `provider.aether.headers` and `provider.aether.options.headers`.
  This prevents a stale explicit header from overriding the selected/rotated
  SDK API key. Noncredential headers, such as trace IDs, are merged and retained;
  headers belonging to other providers are not touched. Both Arca header
  locations must be JSON objects when present, including in the server response.
  The server's complete `models` object replaces the Arca catalog so removed
  or revoked models do not linger. If top-level `model` or `small_model` refers
  to `aether/<id>` absent from the new catalog, only that invalid reference is
  deleted and a brief notice names the affected setting, not its value or key.
  OpenCode can then select a model again; the sync never chooses a replacement
  on your behalf. Valid Arca defaults and references to other providers are
  preserved.
- `opencode.json.bak.*` (0600) alongside the client config before each changed
  rewrite. Each backup contains the exact previous bytes. Unchanged syncs do not
  create another backup. Backups may contain old API keys: keep them private and
  remove obsolete backups according to your credential-retention policy.
- `~/.aether/opencode.json` (0600), storing the private API key, gateway URL and
  the chosen destination path. This is separate from `client.env` and is not
  overwritten by installing another client. Rotate the key by generating and
  running a new OpenCode installation command for the replacement key.
- `~/.aether/opencode-sync.py` (0700), a persistent stdlib-only Python 3 sync
  helper with no embedded API key. `~/.aether` is restricted to 0700. A private
  lock file serializes simultaneous Arca sync runs.

After setup, run this same command whenever the gateway's model catalog or key
permissions change:

```sh
python3 "$HOME/.aether/opencode-sync.py"
```

The helper reads its private local credentials, fetches the current config and
repeats the same merge/backup/atomic-write flow. It does not need another install
code, shell exports, CC Switch, or a key-bearing URL. The original install code
remains single-use. There is no background polling: rerun the sync command and
restart OpenCode to load changes. Project-level OpenCode configuration and
`OPENCODE_CONFIG`/`OPENCODE_CONFIG_DIR` overrides are not edited and can take
precedence over the global file. See the [OpenCode configuration docs](https://opencode.ai/docs/config/).

The merge requires strict JSON objects. Empty or damaged JSON, duplicate keys,
non-standard constants, invalid provider/options/headers types, and an existing sibling
`opencode.jsonc` cause a nonzero exit **without overwriting the config or existing
sync credentials/helper**. JSONC comments/trailing commas are not stripped or
silently converted, even though OpenCode itself supports JSONC. Convert and
validate your own JSONC configuration deliberately before rerunning setup.
Symlinked destination files, sync credentials and immediate configuration
directories are also refused. Network/TLS/auth failures, unsafe redirects and
invalid catalog responses leave the client config unchanged. Errors do not print
response bodies, credential values or Python tracebacks. The fetched installer
contains the key in memory: do not record its source or execute it with an
external shell debugger/transcript that captures source content.

Offline regression test (Python 3.11+ for the existing Codex TOML assertions):

```sh
python3 tests/linux_client_setup_smoke_test.py
```

The test uses temporary homes, fake client binaries, synthetic credentials and
a loopback-only mock gateway; it does not contact a real gateway or install npm
packages. It covers mixed-case credential-header cleanup during key rotation,
trace-header preservation, invalid-header failure safety, and clearing only
unavailable Arca `model`/`small_model` defaults when the catalog shrinks.

## Remote Control Removed

The application no longer ships the browser remote-control UI, companion VS Code
extension, cloud relay, or their build/release jobs. Old `/dashboard/vscodex`
bookmarks redirect to API Keys. Legacy remote-control API and asset paths return
HTTP 410 without contacting a sidecar. Existing deployments must deploy this
updated application; separately running legacy relay containers are not stopped
by a source-code change. No production containers or user databases are changed
by this removal.

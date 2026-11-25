# Arca integration for New API

The embedded Vue console opens at `/arca/`. The same gateway serves the New API login, channels, models, keys, billing, and original console routes. The Arca console uses the same authenticated account and delegates sign-in, key creation, and advanced operations to the native interface.

## Build and verification

Use `make build-all-web` before building the Go binary. The Dockerfile also embeds both frontends. The Arca console is a separate Vite build nested at `web/dist/arca`. The order matters because the original frontend build replaces `web/dist`.

```sh
make build-all-web
go build .
go test ./...
cd web-arca
node --test src/api.test.js
```

Use isolated databases for the MySQL and PostgreSQL matrices. The Arca Go tests document their `TEST_*_DSN` environment variables. The diagnostics router test also accepts an isolated ClickHouse log database.

## Main features

| Feature | Entry point |
|---|---|
| Feedback, edits, replies, pinning and diagnostics | `/arca/#feedback` and `/api/arca/feedback` |
| In-app mentions | Bell in the Arca console |
| External mentions | Existing administrator notification preferences, with a persistent delivery queue |
| User profiles, avatars and token history | `/arca/#profile` |
| Team activity and member anonymity | `/arca/#activity` |
| Organization and model-family policy | `/arca/#entitlements` for the root role, plus the personal overview |
| OpenCode configuration | The key screen or `GET /api/opencode/config` with the applicable relay key |
| Linux and macOS setup | `GET /api/opencode/setup` downloads the interactive synchronization tool |
| OpenCode Go quota monitoring | Built-in `opencode-go` provider plugin and per-credential balance display |
| Arca MCP | `POST /mcp` using a scoped management PAT |

For a deployed MCP endpoint, set `ARCA_MCP_ALLOWED_HOSTS` to the gateway hostnames. The default allows loopback hosts. Browser clients also require an explicitly allowed origin. Mint PAT scopes using the New API account interface. MCP feedback writes require a preview and single-use confirmation. The tool list advertises the actions permitted for the token's current account.

External mentions follow the administrator's configured New API SMTP or push preference. Configure the public address so notification links resolve to the gateway. `ARCA_FEEDBACK_DELIVERY_ENABLED` controls delivery. For an optional ServerChan recipient, set `ARCA_FEEDBACK_SERVERCHAN_USER_ID` and supply `ARCA_FEEDBACK_SERVERCHAN_SENDKEY` through deployment secrets.

## Current release boundaries

The budget ledger uses New API's configured gateway pricing. An organization can fund usage without a member's wallet. Realtime and legacy Midjourney requests subject to organization funding or finite budgets are rejected before dispatch. Provider invoice reconciliation uses a separate cost basis and needs a source of actual provider cost.

Ordinary requests with an explicit final usage record can reconcile a missed budget settlement. For a task with a known terminal charge or refund, the organization ledger and terminal status transition commit together. When the provider gives insufficient evidence for a different charge, the ledger retains the submitted estimate. The native wallet and token adjustments remain under the New API settlement flow.

Quota polling can proactively disable an exhausted OpenCode Go credential and allow healthy ones to serve subsequent requests. An upstream model-specific refusal does not globally disable a credential. Immediate same-request fallback across multiple keys after a just-observed refusal depends on native retry behavior.

The Arca console implements core user and support workflows. The original console remains the entry point for provider administration, subscriptions, plugin management, and account-security operations. The organization editor accepts the versioned policy document and previews one subject at a time. The compiled UI uses Arca's paper palette and shared product terminology.

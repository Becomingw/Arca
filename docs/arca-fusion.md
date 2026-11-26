# Arca integration for New API

Arca is presented by the New API React application. The homepage, authentication, personal workspace and management screens use the same router, session, query cache, theme and component library. The server embeds the standard `web/dist` output.

## Build and verification

```sh
make build-web
go build .
go test ./...
cd web
bun run test
```

The normal Docker and release builds produce the same frontend. Use isolated databases for the MySQL and PostgreSQL tests. The Go tests document their `TEST_*_DSN` environment variables. The diagnostics router test also accepts an isolated ClickHouse log database.

## Main features

| Feature | Entry point |
|---|---|
| Arca homepage, authentication and available models | `/`, `/sign-in`, `/workspace` |
| Gateway administration | `/dashboard` for administrators |
| Feedback, edits, replies, status and request diagnostics | `/feedback`, including feedback from a user’s own usage log details |
| In-app mentions | Discussions tab in the shared notification center |
| External mentions | Configured administrator notification channels and a persistent delivery queue |
| Avatars, token heatmap and activity privacy | `/profile` |
| Team activity | `/team-activity` |
| Organizations, model families and budget policy | `/entitlements` for the root role, with a personal summary in the workspace and profile |
| OpenCode and CC Switch client setup | Native API key actions at `/keys` |
| Linux and macOS setup | The API key screen and `GET /api/opencode/setup` |
| OpenCode Go quota monitoring | Built-in `opencode-go` provider plugin and per-credential balance display |
| Arca MCP | Native `/security` configuration and scoped PAT authentication at `POST /mcp` |

The former `/arca/` bookmarks resolve to the matching native pages, including feedback topics. Existing application data, configuration and API routes are reused when upgrading the prior fusion branch.

For a deployed MCP endpoint, set `ARCA_MCP_ALLOWED_HOSTS` to the gateway hostnames. The default allows loopback hosts. Browser clients also require an explicitly allowed origin. Mint PAT scopes using the New API account interface. MCP feedback writes require a preview and single-use confirmation. The tool list advertises the actions permitted for the token's current account.

External mentions follow the administrator's configured New API SMTP or push preference. Configure the public address so notification links resolve to the gateway. `ARCA_FEEDBACK_DELIVERY_ENABLED` controls delivery. For an optional ServerChan recipient, set `ARCA_FEEDBACK_SERVERCHAN_USER_ID` and supply `ARCA_FEEDBACK_SERVERCHAN_SENDKEY` through deployment secrets.

## Current release boundaries

The budget ledger uses New API's configured gateway pricing. An organization can fund usage without a member's wallet. Realtime and legacy Midjourney requests subject to organization funding or finite budgets are rejected before dispatch. Provider invoice reconciliation uses a separate cost basis and needs a source of actual provider cost.

Ordinary requests with an explicit final usage record can reconcile a missed budget settlement. For a task with a known terminal charge or refund, the organization ledger and terminal status transition commit together. When the provider gives insufficient evidence for a different charge, the ledger retains the submitted estimate. The native wallet and token adjustments remain under the New API settlement flow.

Quota polling can proactively disable an exhausted OpenCode Go credential and allow healthy ones to serve subsequent requests. An upstream model-specific refusal does not globally disable a credential. Immediate same-request fallback across multiple keys after a just-observed refusal depends on native retry behavior.

The organization editor previews one user and optional API key at a time. The configured theme, language preference and authentication methods continue to apply across the application.

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
| Team activity, working patterns and common models | Signed-in Team tab at `/rankings` |
| Managed subscriptions, model families and budget policy | `/entitlements` for the root role, with a personal summary in the workspace and profile |
| Temporary usage grants | Administrator codes at `/redemption-codes` and member redemption at `/profile` |
| OpenCode and CC Switch client setup | Native API key actions at `/keys` |
| CC Switch subscription usage | Key-authorized `GET /api/ccswitch/usage`, attached to eligible CC Switch imports |
| Linux and macOS setup | The API key screen and `GET /api/opencode/setup` |
| OpenCode Go quota monitoring | Built-in `opencode-go` provider plugin and per-credential balance display |
| Arca MCP | Native `/security` configuration and scoped PAT authentication at `POST /mcp` |

The former `/arca/` bookmarks resolve to the matching native pages, including feedback topics. Earlier team activity bookmarks also open the signed-in team ranking. Existing usage history and application records remain available when upgrading the prior fusion branch.

## Managed access

A user-specific plan takes precedence over the plan for the user's native group. One plan supplies limits and model access for a request. Existing Arca policies remain the fallback when neither assignment is present. Positive-cost requests without applicable plan allowance require compatible supplemental usage. Paid subscriptions, customer balance operations, checkout and payment webhooks are retired. Existing accounting tables remain available for upgrades and historical settlements.

An unused code grants one account a finite amount of supplemental usage. An unrestricted grant can fund any model that account and key can already use. A scoped grant can be limited to exact aliases or to a snapshot of one model family. Code redemption deadline, optional usage dates, and usable lifetime after activation are separate controls. The gateway reserves plan capacity before a grant and prefers grants that expire earlier. Redemption, reservation, and settlement serialize on the user's budget account.

External mentions follow the administrator's configured New API SMTP or push preference. Configure the public address so notification links resolve to the gateway. `ARCA_FEEDBACK_DELIVERY_ENABLED` controls delivery. For an optional ServerChan recipient, set `ARCA_FEEDBACK_SERVERCHAN_USER_ID` and supply `ARCA_FEEDBACK_SERVERCHAN_SENDKEY` through deployment secrets.

## CC Switch subscription usage

Open the key's CC Switch import action at `/keys`. The key's user quota query setting controls access to the subscription view. CC Switch queries the gateway with that inference key and shows its owner's effective subscription, its applicable windows, and temporary usage grants on separate lines. Imports enable 30-minute refresh. Calendar windows carry their actual reset dates. The five-hour window rolls as earlier usage ages out. Configure a same-origin HTTPS endpoint for team access. Exact loopback HTTP addresses support local and SSH-forwarded previews.

## Agent operations

Connect an MCP client to `POST /mcp` with a scoped access token created at `/security`. For a deployed endpoint, set `ARCA_MCP_ALLOWED_HOSTS` to the gateway hostnames. The default allows loopback hosts. Browser clients also require an explicitly allowed origin. The tool list reflects the token's scopes and the current role of its owner.

Agent reads include member and group lookups, subscription plans, a member's effective usage allowances, channel status and model routing, plus request diagnostics and feedback. Subscription assignment and clearing require a superadministrator token with entitlement write access. Preview the exact target and change before confirming the current policy version with the single-use confirmation. Feedback writes use the same preview and confirmation workflow.

Use `arca_subscription_plans_list` for plan IDs and the policy version. In `arca_subscription_assign`, include `expected_version`, `subject_type`, the matching `user_id` or `group`, and `plan_id`. An empty plan ID clears the direct assignment. Resubmit the same body with `dry_run: false` and the confirmation from the preview. A concurrent policy change requires a new preview.

## Current release boundaries

The budget ledger uses New API's configured gateway pricing. An organization can fund usage without a member's wallet. Realtime and legacy Midjourney are unavailable through managed entitlements because they use the retired wallet settlement path. Provider invoice reconciliation uses a separate cost basis and needs a source of actual provider cost.

Ordinary requests with an explicit final usage record can reconcile a missed budget settlement. For a task with a known terminal charge or refund, the organization ledger and terminal status transition commit together. When the provider gives insufficient evidence for a different charge, the ledger retains the submitted estimate. API key limits remain under the native New API settlement flow.

Quota polling can proactively disable an exhausted OpenCode Go credential and allow healthy ones to serve subsequent requests. An upstream model-specific refusal does not globally disable a credential. Immediate same-request fallback across multiple keys after a just-observed refusal depends on native retry behavior.

The organization editor previews one user and optional API key at a time. The configured theme, language preference and authentication methods continue to apply across the application.

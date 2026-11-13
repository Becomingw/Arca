# Effective Entitlements and Family Pools

## Rules

`internal_access_policy` stores optional `model_families`, keyed by a stable ID,
with a display `name` and an explicit list of public model `models`. It supports
32 definitions, each with 1..256 unique exact names. These are quota families,
not the upstream/provider family metadata and not name-prefix matching.

Each allocation can have `families: {familyId: ModelAllowance}` alongside exact
`models` sub-pools. Their combined count is at most 15. Both support USD limits
for rolling five hours, day, calendar week (Monday) and calendar month. Totals
support rolling five hours, week and month. Calendar boundaries use the policy
timezone. Empty limits are unbounded, not zero allowances.

- A personal allocation replaces the entire group allocation; missing personal
  fields do not fall back to group caps. Unlimited removes all quota caps.
- Without personal configuration, assigned group allowances are combined using
  the existing most-permissive grant rules. With no assigned allowance, use default.
- Model access remains the separate personal/group model policy. Family membership
  never grants permission to use a model.
- Every matching family and model sub-pool and every total window must allow the
  request. Overlapping family membership does not allow choosing a cheaper pool.
- The settlement ledger stores one reservation per request with the public model
  alias. Pool checks select that same row; there is no extra balance or double debit.
- Current family members' existing active-window history counts, including requests
  admitted before editing the family. Renaming a family does not change its ID.
  Renaming a public model does not automatically rename member aliases or old rows.
- Pending, non-expired reservations count along with actual finalized cost. Released
  and expired pending reservations do not. Reservation estimates are reconciled
  against actual cost at completion; these are not hard upstream spend guarantees.
- The effective allocation is bounded to 64 enforcement windows; exceeding it fails
  closed rather than silently omitting restrictions.

No schema migration is added for families: PostgreSQL selectors use `model = ANY`
over the existing ledger. All pools share the existing per-user admission lock.

## Cost admission estimates

Text input reservations use a heuristic of four ASCII bytes or two non-ASCII UTF-8
bytes per token, including JSON structure and tool definitions. Media parts retain
their separate fixed token budgets. The estimate is capped by the provider-model's
positive `context_limit`, falling back to the global model's limit. This is neither
an exact tokenizer nor context-length validation; upstream limits still apply and
actual reported usage determines settlement. Output defaults and hosted-tool
surcharges remain separate.

OpenAI Chat/Responses reservations consider uncached input and cache reads, but
do not invent a cache-write charge from shared pricing catalogs. Claude and other
formats retain their cache-write scenarios. Actual billing rules are unchanged.

Cost-limit errors include `details.used` (finalized plus active reserved USD) and
`details.estimated` (this request's estimated USD). For rolling windows,
`retry_after` uses the first ledger release that frees enough capacity, considering
both window expiry and pending-reservation expiry. It is advisory: other usage or
settlement can change availability. Fixed windows retain their reset deadline; if
the request estimate itself exceeds the whole limit, waiting cannot make it fit
and the fallback window duration is not a promise of admission.

## Read-Only Preview

`POST /api/admin/access-preview` accepts:

```json
{"user_id":"subject-id","api_key_id":"optional-owned-key-id","config":{}}
```

`config`, if present, is the **complete proposed internal policy**, not a patch.
It is validated with the same parser as Save. The response includes saved/proposed
effective allocations, source, group IDs, exact family members, ledger usage,
reserved amount and remaining USD for each window. No configuration is saved,
credential revealed, inference dispatched or model permission changed.

User permissions and optional key restrictions come from the authentication policy
resolver. Only the selected user's owned key is accepted. Null model restrictions
mean unbounded; an empty list means deny all. This is a policy preview, not proof
that an upstream is online or that rate/IP/concurrency checks will pass later.
The response's `enabled` flag indicates whether internal quota enforcement is active.
Storage failures return unavailable, never invented zero usage.

Super, audit and user administrators require `admin:users:read`. Service admins and
ordinary users cannot preview other users. The POST is classified as read-only;
`admin:users:write` alone is not a read grant. MCP exposes the identical operation as
`arca_benefits_preview`, accepting a body but no write confirmation ticket.

The frontend calls the same `buildDraft()` used by Save, including changes to other
subjects and definitions. Editing or switching the subject invalidates the displayed
preview; old asynchronous responses cannot replace the latest selection. Save still
uses the existing expected-value CAS guard and may conflict after a preview.

Personal profiles display family pools separately from total windows.

## Local Verification

Run focused tests for `internal_access`, `internal_families`, `api::access_preview`,
`api::account`, `api::mcp` and the settlement/account-usage repositories. The isolated
PostgreSQL lifecycle test `postgres_internal_model_quotas_are_atomic_and_isolated`
includes concurrent requests alternating between two family members; exactly three
10-unit admissions may enter a 30-unit family pool, recorded as three ledger rows.
Account-usage SQL tests verify matching members sum once, tenant isolation, expiry
and exclusive boundaries. Never point these tests at production.

Recorded local acceptance (2026-10-16): entitlement parser tests 5 passed, policy
compiler/admission tests 13 passed, in-memory cost settlement tests 8 passed, and
the opt-in PostgreSQL account-ledger test (including 16-way shared-family admission)
1 passed. The lifecycle test's automatic PostgreSQL startup hit a system shared-memory
limit and skipped; that skip is not evidence. The direct adapter test instead used
the existing loopback-only test server and its own random schema, which was removed.
Final API/MCP regression passed 47 tests; the separately enabled compatibility
PostgreSQL persistence/CAS test also passed. Frontend full suite passed 1,700 tests,
with type-check/build and desktop/mobile visual checks. No production changes or
real upstream inference were made.

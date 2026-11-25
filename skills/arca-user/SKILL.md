---
name: arca-user
description: Help Arca users connect Codex, Claude Code, or OpenCode to the New API gateway, check their API key's available models, and submit feedback using sanitized request diagnostics.
---

# Arca user guide

Use the base address shown in the user's Arca console. The gateway's response header `X-Oneapi-Request-Id` identifies a request for support. Keep inference keys and management credentials out of conversations and public discussions.

## Client setup

- An OpenAI-compatible client uses the gateway's `/v1` base and a personal inference key. Codex uses the Responses API. Claude Code uses the gateway root as its Anthropic base address.
- Model IDs from `GET /v1/models` reflect the current inference key and group. Use the exact ID in the client. Current gateway routing and entitlement checks apply to every call.
- OpenCode's own provider config is available at `GET /api/opencode/config`, authenticated by the selected inference key. The downloaded JSON contains that key's Chat Completions model aliases and no credential. Set the key in the client's private credential store.
- In the New API key list, open CC Switch for the selected key, choose OpenCode, then choose a model and click the import button. The browser sends the chosen key to the CC Switch application as part of the import.
- On Linux or macOS, download `/api/opencode/setup` from the trusted gateway. Run `python3 opencode-sync.py --gateway <ARCA_URL>` in a terminal and enter your inference key at the hidden prompt. It updates the managed provider in the global OpenCode configuration and writes private local sync credentials. Run `python3 ~/.aether/opencode-sync.py` to refresh the catalog later.

## Feedback

A support topic should state the symptom, expected result, model, client, time and the response request ID. All logged-in users can read feedback. The gateway can attach a server-generated diagnostic summary for a request owned by the author. The summary labels evidence and unknowns. It does not establish a final outcome from a billed or failed attempt alone.

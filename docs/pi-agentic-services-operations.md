# Pi agentic services pilot: operations and evaluation

This is a proof of concept. The browser talks to ChenWeb; ChenWeb owns sign-in, conversation records, source access, prompts, and the public response stream. A private Pi gateway runs the model and calls only five read-only ChenWeb knowledge tools. Do not expose the gateway or its internal tool routes through a public proxy.

## Start the pilot

1. Configure ChenWeb's normal database and sign-in environment. ChenWeb applies the project goose migrations on startup; the three new agentic migrations create conversation/audit tables, explicit knowledge grants, and a one-running-turn guard.
2. Set `PI_GATEWAY_SECRET` to the same long random secret for ChenWeb and Pi, and set a **different** `PI_RUN_CAPABILITY_SECRET` of at least 32 bytes for ChenWeb. Keep both out of logs and source control. If either is absent, conversation runs or knowledge tools fail closed.
3. Configure provider credentials in Pi's environment for the selected model. The two default profiles use Anthropic; set up a credential that Pi's model runtime accepts before testing a live turn. Set `PI_KNOWLEDGE_GUIDE_PROVIDER` / `_MODEL` and `PI_PROBLEM_DIAGNOSTICS_PROVIDER` / `_MODEL` independently if another provider or model is intended.
4. Set `PI_GATEWAY_DIR` to the absolute `ThirdParty/pi` directory. From ChenWeb, run `mise dev-agent-services`. This launches ChenWeb's API and web page plus `bun run gateway` in the Pi directory. Stop the foreground task with Ctrl-C; the child processes stop with it.
5. Grant the pilot user access to the intended knowledge store (below), sign in, and open **Workspace → Knowledge Desk** or `/home3/agent-services`.

Pi binds to `127.0.0.1:4317` by default. ChenWeb uses `PI_GATEWAY_URL=http://127.0.0.1:4317` by default; Pi uses `CHENWEB_INTERNAL_URL=http://127.0.0.1:1323` by default. Keep `PI_GATEWAY_HOST=127.0.0.1` for the pilot. If ports change, set the matching URL/port variables in both processes. The Pi health endpoint is private: `GET /health` requires `Authorization: Bearer <PI_GATEWAY_SECRET>`. ChenWeb's signed-in `/api/v1/agent-services/health` checks ChenWeb routing, not the gateway's model credentials.

The prompts live in `prompts/prompt-agent-knowledge-guide-v1.md` and `prompts/prompt-agent-problem-diagnosis-v1.md`; `PROMPT_DIR` overrides the prompt directory. Each slug has its own `PI_KNOWLEDGE_GUIDE_*` or `PI_PROBLEM_DIAGNOSTICS_*` settings for enabled state, pilot user IDs, provider/model disclosure, allowed store names/document groups, ask/auto default, and tool/time/output/evidence limits. Profiles are loaded at ChenWeb startup, so restart after changing them. The page tells users which model provider receives messages and retrieved evidence; verify that disclosure matches the actual deployment.

## Grant and revoke knowledge access

Granting is an explicit operator action for this pilot; a service being visible does **not** itself grant knowledge access. First verify the signed-in user ID, the exact numeric `kb.knowledge_store.id`, its tenant, and whether the intended grant is store-wide or for one `kb.inputs.id`. Do not grant by store name alone when names may occur in multiple tenants.

For a short-lived store-wide pilot grant, replace the placeholders only after that verification:

```sql
INSERT INTO kb.agentic_knowledge_grants
  (user_id, knowledge_store_id, document_id, active, expires_at)
VALUES ('<verified-user-id>', <verified-store-id>, NULL, TRUE, now() + interval '7 days')
ON CONFLICT (user_id, knowledge_store_id) WHERE document_id IS NULL
DO UPDATE SET active=TRUE, expires_at=EXCLUDED.expires_at, updated_at=now();
```

For a single document, supply its verified ID instead of `NULL`; it must belong to the same store. To revoke, update only the exact user's grant row(s) to `active=FALSE`. The next tool call rechecks the grant. Opening a saved conversation also rechecks each cited source and hides an answer if its source was revoked, moved, deleted, or reprocessed; hidden text is not sent back to Pi as history. A document-specific grant alone may not permit store-wide search, because search cannot safely inspect every document first.

## Limits, saving, and failures

ChenWeb pins a conversation to its service slug, profile version, and model. Only one turn may run in a conversation at a time. The browser can choose automatic use of already-allowed read tools or approval before each tool call; it can stop a run. Pi has no built-in shell, edit, or network tools in this gateway. Tool inputs, evidence size, tool-call count, elapsed time, answer size, and emitted event types are bounded on both sides.

Conversation messages, audit summaries, citations, usage, and feedback are saved in ChenWeb's project database. There is no automatic conversation-retention job in this pilot; the user can delete a conversation, which cascades to its saved messages, source links, tool audits, attempts, and feedback. Partially streamed answers may remain marked `streaming` after a stop, browser disconnect, or gateway failure; the page refetches ChenWeb's saved state. Tool arguments/passages, hidden model reasoning, and provider credentials are not saved in the agentic audit tables or sent to the browser.

If a turn cannot start, check in this order: signed-in user and pilot availability, an active exact grant, both secrets, gateway health and matching ports, provider/model availability in Pi, and ChenWeb's project migration log. A `409` means an idempotent turn or another active turn already exists; `403` usually means the current user lacks a scoped grant. A stream ending without a final ChenWeb completion is interrupted, not a successful answer. Source-dependent answers are only marked complete after ChenWeb verifies and saves their citations.

## Pilot evaluation

Use `server/api/agentservicehandler/testdata/evaluation-cases.json` with both guides. For each scenario, prepare the described accessible or inaccessible documents, run the question as a pilot user, and record the answer, citations, activity, and any approvals. Check normal, ambiguous, missing-evidence, conflicting-source, hostile-document, access-denied, and access-revoked-after-save cases. The expected behaviors are safety checks, not exact words for the model to repeat. Keep a human reviewer in the loop before calling the pilot suitable for consequential decisions.

Automated checks: `cd ThirdParty/pi && bun test && bun run check`; `cd ChenWeb/web && bun test && bun run check && bun run build`; `cd ChenWeb && mise build-web && go test ./... && mise build-server`. The generated static frontend is embedded from `server/api/webbuild`; `mise build-web` copies it after a successful build. Run goose up/down/up only against a **dedicated empty development database**: the down steps delete agentic tables/data. If no isolated database is available, rely on the migration contract tests and state that the live migration cycle was not run.

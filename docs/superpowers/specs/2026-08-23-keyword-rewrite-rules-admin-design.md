# Keyword Rewrite Rules Admin Page Design

## Goal

Add an embedded administration page at **Development → System Admin → Keyword Normalization → Rewrite Rules** for creating, reading, updating, enabling, and disabling rows in `kb.keyword_rewrite_rules`. Rules are never deleted from this page. After the page is implemented and verified, create the English Markdown user manual named `keyword-rewrite` and show that manual name in the page header because ChenWeb does not yet have a user-manual route.

## Existing Behavior

Tier 3 receives one raw surface string after tiers 0–2 miss. It loads every enabled rule whose `scope` exactly matches the request, ordered by `rule_id`, then compares the entire raw input to each rule's `pattern` with Go string equality. The first match replaces the entire input, stops the loop, normalizes the replacement, and retries tiers 0–1. It does not tokenize, query by pattern, scan substrings, evaluate regular expressions, or chain rules.

The existing API only creates rules, lists enabled rules for a scope, and toggles `enabled`. The existing store can also read one rule. A true administration page therefore needs list-all and update support, but not delete support.

## Navigation and Integration

Add this three-level branch to the existing hardcoded `NavRail` tree:

- System Admin
  - Keyword Normalization
    - Rewrite Rules

Use stable IDs `sysadmin-keyword-normalization` and `sysadmin-keyword-rewrite-rules`. Add matching English and Simplified Chinese `kb.page_config` entries in a new goose migration so the `/development` configuration overlay recognizes and can govern both nodes. Seed both language rows with non-null `access_role = ["admin", "root"]`, `accessible = true`, and `enabled = true`; the down migration removes only these two entry keys. Render the new view through `content-panel.svelte`, following the other embedded System Admin pages rather than adding a standalone SvelteKit route. Page visibility is presentation only and never substitutes for handler authorization.

## Backend Design

Keep runtime resolution and administration separate:

- Preserve `ListEnabledRules(ctx, scope)` unchanged for Tier 3.
- Add a store method that lists all rules, optionally filtered by exact scope, ordered predictably by `rule_id`. Preserve `ListEnabledRules(ctx, scope)` unchanged for runtime callers.
- Add a store method that updates `pattern`, `replacement`, `scope`, `enabled`, and `provenance` while leaving `rule_id` immutable.
- Add a store query that returns distinct nonblank scopes from keyword concepts, keyword surfaces, and rewrite rules; always include `_`.
- Reuse one typed validation path for both create and update. `rule_id`, `scope`, and `provenance` are trimmed. `pattern` and `replacement` are preserved byte-for-byte because whitespace can affect exact matching, but values that are empty or whitespace-only are rejected. Create retains the existing defaults (`scope = "_"`, `provenance = "human:"`, `enabled = false`) when those optional fields are omitted. Update is a full replacement of editable fields and rejects missing/blank required strings.
- Define `ErrRewriteRuleInvalid` and `ErrRewriteRuleNotFound` (or typed equivalents). Store update/toggle converts zero affected rows to not-found. The handler recognizes PostgreSQL SQLSTATE `23505` from duplicate create as conflict; all other database errors remain internal failures.

Preserve the existing runtime-compatible endpoint:

- `GET /keyword-rewrite-rules?scope=<scope>` continues to list only enabled rules for one exact scope. Its contract and `ListEnabledRules` call remain unchanged.

Expose page administration with explicit admin semantics:

- `GET /keyword-rewrite-rules/admin?scope=<optional>` lists all enabled and disabled rules, optionally filtered by exact scope.
- `GET /keyword-rewrite-rules/admin/scopes` returns known scopes.
- `POST /keyword-rewrite-rules` creates a rule. The request accepts optional `scope`, `provenance`, and `enabled`; omitted values receive the existing defaults.
- `PUT /keyword-rewrite-rules/:rule_id` updates every editable field. Its accepted schema deliberately omits `rule_id`, and JSON decoding rejects unknown fields, so a body attempting to change identity fails validation.
- `PUT /keyword-rewrite-rules/:rule_id/enabled` remains available for the table's quick toggle.

All administration reads and writes require an authenticated owner/admin (`UserInfo.IsOwner`, `UserInfo.Admin`, or case-insensitive `admin`/`root` role) inside the handler; authenticated non-admin users receive `403`, and unauthenticated callers receive `401`. The legacy enabled-only GET retains its current authenticated-route behavior. Do not add a DELETE endpoint.

JSON contracts are:

- list success `200`: `{"status":true,"results":[<rule>],"total":N}`; an empty list is `[]`, never `null`;
- scopes success `200`: `{"status":true,"results":["_",...],"total":N}`;
- create success `201`: `{"status":true,"record":<rule>}`;
- update success `200`: `{"status":true,"record":<rule>}`;
- toggle success `200`: `{"status":true,"record":<rule>}`.

Create/update use strict JSON decoding. Create's `enabled` is `*bool` so omission is distinguishable from explicit false. Update requires all editable fields, including `enabled`. Toggle accepts only `{"enabled":<bool>}` with a required pointer boolean; missing, malformed, trailing, or unknown data returns `400` rather than silently disabling. Map typed validation errors to `400`, typed/sql no-row results to `404`, SQLSTATE `23505` to `409`, and unexpected failures to `500`. Log failures with the existing request logger and unique handler location codes.

## Page Design

The page uses the existing ChenWeb dark/light tokens and embedded admin spacing. Its top card contains:

- title: `Rewrite Rules`;
- a plain-language description that these are exact whole-input, scope-specific aliases used by Tier 3 before deterministic retry;
- the text `User manual: keyword-rewrite` (not a hyperlink yet);
- `New Rule` and `Refresh` actions.

Below it, provide client-side search over rule ID, pattern, replacement, and provenance, plus scope and enabled-state filters. The table shows rule ID, pattern, replacement, scope, status, provenance, modified time, and an Edit action. Empty, loading, and error states remain inside the table card.

Create and Edit open a modal. The left column contains:

- immutable rule ID after creation;
- pattern;
- replacement;
- scope selector;
- provenance;
- enabled checkbox.

The scope selector offers known scopes, defaults to `_`, and includes `Custom…`, which reveals a required free-text value. Enabled rules may be edited directly.

The modal's right column is a live, explicitly **hypothetical** behavioral preview. For a new blank rule it shows an instructional empty state. As fields are entered, it shows raw input → rewritten input, the exact scope, enabled state, and a warning that matching is whole-string, case-sensitive, single-rule, and non-regex. Editing an existing rule populates this preview immediately. A disabled draft says it will not run until enabled. If the currently loaded rules contain an enabled rule with the same pattern and scope and a lexically smaller `rule_id`, the preview warns that the draft would be shadowed because runtime rules are ordered by `rule_id` and first match wins. The preview illustrates behavior only; it does not call the resolver or claim that the replacement currently resolves to a concept.

There is no delete action. Disabling is the preservation mechanism and is available both in the modal and as a deliberate table toggle.

## Client Design

Create a focused TypeScript client module containing the rule/scope types and fetch functions. It must normalize API errors into user-readable messages and keep view code free of endpoint construction. The Svelte view owns presentation state, filtering, modal drafts, validation messages, confirmation for enabled-state changes, and refresh-after-write behavior.

Minimum client-side validation:

- rule ID, pattern, replacement, scope, and provenance must be nonblank after presence checks; pattern/replacement bytes are not silently trimmed when submitted;
- pattern cannot contain `(`, `)`, or `\`, matching current server validation;
- a custom scope cannot remain blank;
- rule ID cannot change during edit.

The server remains authoritative and repeats all applicable validation.

## Verification

Backend tests cover:

- unchanged legacy enabled-only listing plus admin list-all and exact-scope filtering;
- update of an enabled rule;
- immutable rule ID;
- shared literal validation on update;
- known-scope deduplication and `_` inclusion;
- owner/admin/root authorization and ordinary-user rejection independently of page visibility;
- strict request decoding, required toggle boolean, defaults, duplicate create, missing update/toggle, empty-list serialization, handler status codes, and exact payload envelopes;
- unchanged Tier 3 enabled-only lookup behavior.

Frontend tests cover client endpoint construction, response parsing, and error handling. Extract pure view-model helpers where useful and test filtering, create/edit draft population, custom-scope validation, hypothetical/disabled/shadowed preview messaging, and absence of any delete operation. Verify nav selection and content-panel rendering with the project's existing source/component test pattern, including the new migration's English/Chinese rows, non-null admin/root roles, and targeted rollback. Manually verify modal focus/close behavior, live preview, enable confirmation, refresh-after-write, empty/error/loading states, and both themes.

Run `bun test src/lib/components/home3/keyword-rewrite-rules-client.test.ts` and any focused view-model/nav tests from `web/`, followed by `bun run check` and `bun run build`. Run focused Go tests for `server/api/ontology/keywords` and `server/api/kbhandler`, route registration tests, then the broader affected server tests. The repo has no generic package test script, so do not document `bun test` without a path as the sole frontend verification.

## Documentation

Only after implementation and verification, invoke the `user-manual-writer` workflow to create:

`KnowledgeStore/doc-repo/user-manuals/keyword-rewrite-v1.0-en.md`

The manual explains what rewrite rules are, when they are appropriate, exact matching and scope behavior, examples, risks, and how administrators create, edit, enable, disable, search, and filter rules on this page. It includes the skill-required metadata and change log. The page displays the manual name `keyword-rewrite`; linking is intentionally deferred until user-manual pages exist.

## Non-Goals

- Regex, substring, token-level, or chained rewriting.
- Rule deletion.
- Automatic rule promotion from reconciliation.
- A generic CRUD framework.
- A standalone page route.
- A user-manual viewer or hyperlink.
- Changing Tier 3's O(number of enabled rules in scope) lookup algorithm.

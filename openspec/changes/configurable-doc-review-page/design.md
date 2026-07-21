## Context

The DB-backed page-config capability (spec `2026072001` §9, ADR `2026072003`)
already provides a page-agnostic resolver (`GET /api/v1/page-config/:pageKey`),
admin API/UI (`/semos/admin/page-config`), and frontend client
(`getPageConfig` in `pageConfigService.ts`). Two pages
(`/home3/knowledge`, `/semos/workspace`) and the `/development` NavRail are
already onboarded. §11 is the normative recipe for onboarding a new page; this
change applies it to the Document Review wizard.

Current state: `document-review-view.svelte` hardcodes all UI chrome in English.
Tiers/aspects already come from the backend (`doc-review.local.toml` via
`listTiers()`/`listAspects()`) and are explicitly out of scope. The component is
not a standalone route — it is mounted by `content-panel.svelte` inside the
`/development` dashboard — so no nav-rail wiring is needed (§11.3 is satisfied by
the `kb.page_def` row alone).

## Goals / Non-Goals

**Goals:**
- Make every page-owned static string in the wizard visibility-toggleable and
  per-language translatable via page-config, keyed by stable `entry_key`.
- Preserve exact current English rendering (fail-open; `en` content `= {}`).
- Add Chinese (`zh-cn`) translations so the page respects the Paraglide locale.
- Zero backend Go changes; reuse the generic resolver and admin surfaces.

**Non-Goals:**
- Migrating tiers/aspects/tier descriptions or aspect chips (stay in TOML).
- Translating dynamic/interpolated fragments (e.g. "{n} aspects selected",
  "Depth {n}", step intro paragraphs) — these stay hardcoded.
- Any change to the page-config backend, tables, or admin UI.

## Decisions

### D1 — `page_key = doc-review`, route `/development`
Stable page key `doc-review`. `kb.page_def.route` records `/development` (where
the view lives) for admin display only; resolution is by `page_key`. Chosen over
`development-doc-review` for brevity and because the entry namespace is already
scoped by `page_key`.

### D2 — Wire the fetch into the component, not a `+page.svelte`
The recipe's Step B assumes a `+page.svelte`, but here the "page" is the
`document-review-view.svelte` component. We add the `getPageConfig` fetch to its
existing `onMount`, hold `let pageConfig = $state<PageConfig | null>(null)`, and
define local `isVisible` / `labelFor` helpers — identical semantics to
`knowledge/+page.svelte`. `getLocale` is imported from `$lib/paraglide/runtime`.

### D3 — Overlay model, fail open, unknown-id warn
Follow §11.2 verbatim: default-visible, hide only `hidden`; `labelFor(id,
default)` = `pageConfig?.overrides[id]?.label ?? default`; `null` config → full
default page. A `$derived` computes `overrides ∪ hidden` keys not in the known
id set and `console.warn`s them (§4.4).

### D4 — Configurable entry inventory (authoritative)
Only the strings the user scoped ("title/subtitle, 4 step-indicator labels, each
step heading, Next/Back/Start Review buttons, field labels, P1–P6 group
labels"). Repeated strings (Next/Back) share one `entry_key`. `en` content is
`{}` (hardcoded default); `zh-cn` carries the translation. Entry ids map to
page-owned concepts, never display text.

| entry_key | EN default (hardcoded) | zh-cn label | entry_desc |
|---|---|---|---|
| `dr-title` | Document Review | 文档审核 | Page title |
| `dr-subtitle` | Submit a document for AI-powered review across quality, compliance, and technical aspects. | 提交文档，进行覆盖质量、合规与技术层面的 AI 审核。 | Page subtitle |
| `dr-btn-next` | Next → | 下一步 → | Wizard "next" button (steps 1–3) |
| `dr-btn-back` | ← Back | ← 返回 | Wizard "back" button (steps 2–4) |
| `dr-btn-start` | Start Review | 开始审核 | Submit button (step 4) |
| `dr-step-select-document` | Select Document | 选择文档 | Step indicator 1 |
| `dr-step-check-level` | Check Level | 审核级别 | Step indicator 2 |
| `dr-step-references` | References | 参考文档 | Step indicator 3 |
| `dr-step-submit` | Submit | 提交 | Step indicator 4 |
| `dr-s1-heading` | Step 1: Select Document | 第一步：选择文档 | Step 1 heading |
| `dr-s2-heading` | Step 2: Choose Check Level | 第二步：选择审核级别 | Step 2 heading |
| `dr-s3-heading` | Step 3: Supporting Documents | 第三步：支持文档 | Step 3 heading |
| `dr-s4-heading` | Step 4: Review Details | 第四步：审核详情 | Step 4 heading |
| `dr-s1-mode-search` | Search Library | 检索文档库 | Step 1 mode toggle: search |
| `dr-s1-mode-upload` | Upload File | 上传文件 | Step 1 mode toggle: upload |
| `dr-s1-parser-label` | Parser | 解析器 | Step 1 parser select label |
| `dr-s1-upload-btn` | Upload & Select | 上传并选择 | Step 1 upload button |
| `dr-s2-depth-label` | Review depth | 审核深度 | Step 2 review-depth field label |
| `dr-s3-add-btn` | Add | 添加 | Step 3 add-reference button |
| `dr-s3-ref-placeholder` | Reference document title or ID | 参考文档标题或编号 | Step 3 reference input placeholder |
| `dr-s4-name-label` | Your Name * | 你的姓名 * | Step 4 requester name label |
| `dr-s4-notes-label` | Notes (optional) | 备注（可选） | Step 4 notes label |
| `dr-s4-report-label` | Report Template (optional) | 报告模板（可选） | Step 4 report-template label |
| `dr-s4-doctpl-label` | Doc Template (optional) | 文档模板（可选） | Step 4 doc-template label |
| `dr-summary-heading` | Review Summary | 审核摘要 | Step 4 summary heading |
| `dr-summary-document` | Document: | 文档： | Summary row: document |
| `dr-summary-checklevel` | Check Level: | 审核级别： | Summary row: check level |
| `dr-summary-aspects` | Aspects: | 审核维度： | Summary row: aspects |
| `dr-summary-depth` | Review Depth: | 审核深度： | Summary row: review depth |
| `dr-summary-requester` | Requester: | 申请人： | Summary row: requester |
| `dr-group-p1` | Language & Style | 语言与文风 | Aspect group P1 label |
| `dr-group-p2` | Structure & Organization | 结构与组织 | Aspect group P2 label |
| `dr-group-p3` | Content Quality | 内容质量 | Aspect group P3 label |
| `dr-group-p4` | Consistency | 一致性 | Aspect group P4 label |
| `dr-group-p5` | Technical & Compliance | 技术与合规 | Aspect group P5 label |
| `dr-group-p6` | Meta & Process | 元信息与流程 | Aspect group P6 label |

The P1–P6 labels stay in the existing `groupLabels` map as hardcoded defaults;
`groupAspectNames` resolves each through `labelFor('dr-group-p{n}', groupLabels[g])`.

**Visibility gating (D4a):** all 36 entries are translatable via `labelFor`.
`isVisible` gating is applied only to entries that render as standalone,
genuinely-optional blocks — `dr-subtitle` and the two optional Step 4 template
fields (`dr-s4-report-label`, `dr-s4-doctpl-label`, each wrapping its
label+input `<div>`). Wizard-critical chrome (nav buttons, step headings,
step-indicator labels, grid-paired summary rows) is translate/rename-only, since
hiding it would break the linear flow or the two-column summary grid. This
mirrors `/semos/workspace`, which gates optional masthead sections with
`isVisible` while translating all content — not every label is a hide target.

### D5 — Access control mirrors the existing seeds
Default-language (`zh-cn`) rows get
`access_role = ["admin","root","guest","dev","k_engineer","trial"]` (current
`[system].access_roles`), `accessible = true`, `enabled = true` — matching the
`home3-knowledge` / `semos-workspace` seeds so all current users keep access.

## Risks / Trade-offs

- **Entry-count churn (36 entries) in one `.svelte` file** → Mechanical
  `labelFor(...)` wrapping only; no logic change. Kept minimal by sharing ids for
  repeated buttons and excluding dynamic fragments.
- **`entry_key` typos are inert** → Mitigated by the §4.4 `console.warn`
  unknown-id diagnostic and by verifying both locales after seeding.
- **Migration re-applied by air on rebuild mid-edit** (per project memory) →
  Seed is idempotent (`ON CONFLICT DO NOTHING`); verify with
  `SELECT ... FROM kb.page_config WHERE page_key='doc-review'` if effects lag.
- **Dynamic strings remain English-only** (e.g. "{n} aspects selected") →
  Accepted; out of scope and avoids brittle interpolation entries.

## Migration Plan

1. Add goose migration `project_migrations/<ts>_seed_page_config_doc_review.sql`
   (page_def + page_config en/zh-cn rows + access_role UPDATE), idempotent, with
   a `-- +goose Down` that deletes `page_key='doc-review'` rows.
2. `mise dev` / air applies it automatically on rebuild; verify in
   `project_db_migration` + a `SELECT` on `kb.page_config`.
3. Frontend edit is additive and fails open — safe to deploy independently; if
   the migration is absent the page simply renders hardcoded English defaults.
4. Rollback: `goose down` removes the seed; the page reverts to hardcoded
   defaults with no code change.

## Open Questions

None — scope, taxonomy handling, and translations are settled.

-- +goose Up
-- Onboard the Document Review wizard (document-review-view.svelte, reached via
-- /development → Applications → 文档审核) onto the DB-backed page-config
-- capability (KnowledgeStore spec 2026072001 §11). page_key = 'doc-review'.
--
-- Content model (same as the existing domains): the "en" rows carry empty
-- content ({}) so English renders from the frontend's hardcoded defaults; the
-- "zh-cn" rows carry the translated labels. Default language is
-- [languages].default = "zh-cn", so the zh-cn row is authoritative for access
-- (set by the UPDATE at the end). Idempotent so air re-running it is safe.
--
-- Scope: page-owned static text only (title/subtitle, step-indicator labels,
-- step headings, Next/Back/Start buttons, field labels, P1–P6 group labels).
-- Tiers/aspects stay sourced from doc-review.local.toml and are NOT seeded here.

INSERT INTO kb.page_def (page_key, route, title, description) VALUES
    ('doc-review', '/development', 'Document Review',
        'Configurable static UI text for the Document Review wizard.')
ON CONFLICT (page_key) DO NOTHING;

-- ── en rows (empty content = hardcoded English default) ──
INSERT INTO kb.page_config (page_key, entry_key, language, content, entry_desc) VALUES
    ('doc-review', 'dr-title',                'en', '{}'::jsonb, 'Page title'),
    ('doc-review', 'dr-subtitle',             'en', '{}'::jsonb, 'Page subtitle'),
    ('doc-review', 'dr-btn-next',             'en', '{}'::jsonb, 'Wizard "next" button (steps 1–3)'),
    ('doc-review', 'dr-btn-back',             'en', '{}'::jsonb, 'Wizard "back" button (steps 2–4)'),
    ('doc-review', 'dr-btn-start',            'en', '{}'::jsonb, 'Submit button (step 4)'),
    ('doc-review', 'dr-step-select-document', 'en', '{}'::jsonb, 'Step indicator 1'),
    ('doc-review', 'dr-step-check-level',     'en', '{}'::jsonb, 'Step indicator 2'),
    ('doc-review', 'dr-step-references',      'en', '{}'::jsonb, 'Step indicator 3'),
    ('doc-review', 'dr-step-submit',          'en', '{}'::jsonb, 'Step indicator 4'),
    ('doc-review', 'dr-s1-heading',           'en', '{}'::jsonb, 'Step 1 heading'),
    ('doc-review', 'dr-s2-heading',           'en', '{}'::jsonb, 'Step 2 heading'),
    ('doc-review', 'dr-s3-heading',           'en', '{}'::jsonb, 'Step 3 heading'),
    ('doc-review', 'dr-s4-heading',           'en', '{}'::jsonb, 'Step 4 heading'),
    ('doc-review', 'dr-s1-mode-search',       'en', '{}'::jsonb, 'Step 1 mode toggle: search'),
    ('doc-review', 'dr-s1-mode-upload',       'en', '{}'::jsonb, 'Step 1 mode toggle: upload'),
    ('doc-review', 'dr-s1-parser-label',      'en', '{}'::jsonb, 'Step 1 parser select label'),
    ('doc-review', 'dr-s1-upload-btn',        'en', '{}'::jsonb, 'Step 1 upload button'),
    ('doc-review', 'dr-s2-depth-label',       'en', '{}'::jsonb, 'Step 2 review-depth field label'),
    ('doc-review', 'dr-s3-add-btn',           'en', '{}'::jsonb, 'Step 3 add-reference button'),
    ('doc-review', 'dr-s3-ref-placeholder',   'en', '{}'::jsonb, 'Step 3 reference input placeholder'),
    ('doc-review', 'dr-s4-name-label',        'en', '{}'::jsonb, 'Step 4 requester name label'),
    ('doc-review', 'dr-s4-notes-label',       'en', '{}'::jsonb, 'Step 4 notes label'),
    ('doc-review', 'dr-s4-report-label',      'en', '{}'::jsonb, 'Step 4 report-template label'),
    ('doc-review', 'dr-s4-doctpl-label',      'en', '{}'::jsonb, 'Step 4 doc-template label'),
    ('doc-review', 'dr-summary-heading',      'en', '{}'::jsonb, 'Step 4 summary heading'),
    ('doc-review', 'dr-summary-document',     'en', '{}'::jsonb, 'Summary row: document'),
    ('doc-review', 'dr-summary-checklevel',   'en', '{}'::jsonb, 'Summary row: check level'),
    ('doc-review', 'dr-summary-aspects',      'en', '{}'::jsonb, 'Summary row: aspects'),
    ('doc-review', 'dr-summary-depth',        'en', '{}'::jsonb, 'Summary row: review depth'),
    ('doc-review', 'dr-summary-requester',    'en', '{}'::jsonb, 'Summary row: requester'),
    ('doc-review', 'dr-group-p1',             'en', '{}'::jsonb, 'Aspect group P1 label'),
    ('doc-review', 'dr-group-p2',             'en', '{}'::jsonb, 'Aspect group P2 label'),
    ('doc-review', 'dr-group-p3',             'en', '{}'::jsonb, 'Aspect group P3 label'),
    ('doc-review', 'dr-group-p4',             'en', '{}'::jsonb, 'Aspect group P4 label'),
    ('doc-review', 'dr-group-p5',             'en', '{}'::jsonb, 'Aspect group P5 label'),
    ('doc-review', 'dr-group-p6',             'en', '{}'::jsonb, 'Aspect group P6 label')
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- ── zh-cn rows (translated labels) ──
INSERT INTO kb.page_config (page_key, entry_key, language, content, entry_desc) VALUES
    ('doc-review', 'dr-title',                'zh-cn', '{"label":"文档审核"}'::jsonb, 'Page title'),
    ('doc-review', 'dr-subtitle',             'zh-cn', '{"label":"提交文档，进行覆盖质量、合规与技术层面的 AI 审核。"}'::jsonb, 'Page subtitle'),
    ('doc-review', 'dr-btn-next',             'zh-cn', '{"label":"下一步 →"}'::jsonb, 'Wizard "next" button (steps 1–3)'),
    ('doc-review', 'dr-btn-back',             'zh-cn', '{"label":"← 返回"}'::jsonb, 'Wizard "back" button (steps 2–4)'),
    ('doc-review', 'dr-btn-start',            'zh-cn', '{"label":"开始审核"}'::jsonb, 'Submit button (step 4)'),
    ('doc-review', 'dr-step-select-document', 'zh-cn', '{"label":"选择文档"}'::jsonb, 'Step indicator 1'),
    ('doc-review', 'dr-step-check-level',     'zh-cn', '{"label":"审核级别"}'::jsonb, 'Step indicator 2'),
    ('doc-review', 'dr-step-references',      'zh-cn', '{"label":"参考文档"}'::jsonb, 'Step indicator 3'),
    ('doc-review', 'dr-step-submit',          'zh-cn', '{"label":"提交"}'::jsonb, 'Step indicator 4'),
    ('doc-review', 'dr-s1-heading',           'zh-cn', '{"label":"第一步：选择文档"}'::jsonb, 'Step 1 heading'),
    ('doc-review', 'dr-s2-heading',           'zh-cn', '{"label":"第二步：选择审核级别"}'::jsonb, 'Step 2 heading'),
    ('doc-review', 'dr-s3-heading',           'zh-cn', '{"label":"第三步：支持文档"}'::jsonb, 'Step 3 heading'),
    ('doc-review', 'dr-s4-heading',           'zh-cn', '{"label":"第四步：审核详情"}'::jsonb, 'Step 4 heading'),
    ('doc-review', 'dr-s1-mode-search',       'zh-cn', '{"label":"检索文档库"}'::jsonb, 'Step 1 mode toggle: search'),
    ('doc-review', 'dr-s1-mode-upload',       'zh-cn', '{"label":"上传文件"}'::jsonb, 'Step 1 mode toggle: upload'),
    ('doc-review', 'dr-s1-parser-label',      'zh-cn', '{"label":"解析器"}'::jsonb, 'Step 1 parser select label'),
    ('doc-review', 'dr-s1-upload-btn',        'zh-cn', '{"label":"上传并选择"}'::jsonb, 'Step 1 upload button'),
    ('doc-review', 'dr-s2-depth-label',       'zh-cn', '{"label":"审核深度"}'::jsonb, 'Step 2 review-depth field label'),
    ('doc-review', 'dr-s3-add-btn',           'zh-cn', '{"label":"添加"}'::jsonb, 'Step 3 add-reference button'),
    ('doc-review', 'dr-s3-ref-placeholder',   'zh-cn', '{"label":"参考文档标题或编号"}'::jsonb, 'Step 3 reference input placeholder'),
    ('doc-review', 'dr-s4-name-label',        'zh-cn', '{"label":"你的姓名 *"}'::jsonb, 'Step 4 requester name label'),
    ('doc-review', 'dr-s4-notes-label',       'zh-cn', '{"label":"备注（可选）"}'::jsonb, 'Step 4 notes label'),
    ('doc-review', 'dr-s4-report-label',      'zh-cn', '{"label":"报告模板（可选）"}'::jsonb, 'Step 4 report-template label'),
    ('doc-review', 'dr-s4-doctpl-label',      'zh-cn', '{"label":"文档模板（可选）"}'::jsonb, 'Step 4 doc-template label'),
    ('doc-review', 'dr-summary-heading',      'zh-cn', '{"label":"审核摘要"}'::jsonb, 'Step 4 summary heading'),
    ('doc-review', 'dr-summary-document',     'zh-cn', '{"label":"文档："}'::jsonb, 'Summary row: document'),
    ('doc-review', 'dr-summary-checklevel',   'zh-cn', '{"label":"审核级别："}'::jsonb, 'Summary row: check level'),
    ('doc-review', 'dr-summary-aspects',      'zh-cn', '{"label":"审核维度："}'::jsonb, 'Summary row: aspects'),
    ('doc-review', 'dr-summary-depth',        'zh-cn', '{"label":"审核深度："}'::jsonb, 'Summary row: review depth'),
    ('doc-review', 'dr-summary-requester',    'zh-cn', '{"label":"申请人："}'::jsonb, 'Summary row: requester'),
    ('doc-review', 'dr-group-p1',             'zh-cn', '{"label":"语言与文风"}'::jsonb, 'Aspect group P1 label'),
    ('doc-review', 'dr-group-p2',             'zh-cn', '{"label":"结构与组织"}'::jsonb, 'Aspect group P2 label'),
    ('doc-review', 'dr-group-p3',             'zh-cn', '{"label":"内容质量"}'::jsonb, 'Aspect group P3 label'),
    ('doc-review', 'dr-group-p4',             'zh-cn', '{"label":"一致性"}'::jsonb, 'Aspect group P4 label'),
    ('doc-review', 'dr-group-p5',             'zh-cn', '{"label":"技术与合规"}'::jsonb, 'Aspect group P5 label'),
    ('doc-review', 'dr-group-p6',             'zh-cn', '{"label":"元信息与流程"}'::jsonb, 'Aspect group P6 label')
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- Grant the current [system].access_roles set to every seeded entry so all
-- existing users retain access. Only fills rows with no access_role yet, so
-- admin edits are preserved on re-run.
UPDATE kb.page_config
   SET access_role = '["admin","root","guest","dev","k_engineer","trial"]'::jsonb
 WHERE page_key = 'doc-review'
   AND access_role IS NULL;

-- +goose Down
DELETE FROM kb.page_config WHERE page_key = 'doc-review';
DELETE FROM kb.page_def    WHERE page_key = 'doc-review';

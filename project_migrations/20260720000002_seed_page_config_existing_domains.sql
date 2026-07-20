-- +goose Up
-- Seed the two existing file-based domains (/home3/knowledge Wiki sidebar and
-- /semos/workspace) into kb.page_def / kb.page_config so they render identically
-- after being rewired onto the DB-backed resolution API.
--
-- Content model: the "en" rows carry empty content ({}) because the current
-- config/*/labels-en.toml files are empty (English comes from the frontend's
-- hardcoded defaults). The "zh-cn" rows carry only the overrides currently in
-- config/*/labels-zh-cn.toml; ids without a zh-cn override get {} and fall back
-- to the hardcoded English default, exactly matching today's rendering.
-- Default language is [languages].default = "zh-cn", so the zh-cn row is the
-- authoritative row for access. Access columns are set on both rows below.

INSERT INTO kb.page_def (page_key, route, title, description) VALUES
    ('home3-knowledge', '/home3/knowledge', 'Knowledge Wiki Sidebar',
        'Configurable Wiki sidebar menu on /home3/knowledge.'),
    ('semos-workspace', '/semos/workspace', 'SemOS Workspace',
        'Configurable masthead and app tiles on /semos/workspace.')
ON CONFLICT (page_key) DO NOTHING;

-- ── home3-knowledge: en rows (empty content = hardcoded English default) ──
INSERT INTO kb.page_config (page_key, entry_key, language, content) VALUES
    ('home3-knowledge', 'kb-search', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-import', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-doc-wiki', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-chunks', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-llm-wiki', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-llm-wiki-v3', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-input-details', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-doc-structure', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-summary-tree', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-summary-graph', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-semantic-projections', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-topic-tree', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-metrics', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-scene-blocks', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-products', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-provision-tree', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-inventory-items', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-object-manager', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-references', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-formulas', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-tables', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-quotations', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-case-studies', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-workflow', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-product-parts', 'en', '{}'::jsonb),
    ('home3-knowledge', 'kb-category-review', 'en', '{}'::jsonb)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- ── home3-knowledge: zh-cn rows (overrides from labels-zh-cn.toml, else {}) ──
INSERT INTO kb.page_config (page_key, entry_key, language, content) VALUES
    ('home3-knowledge', 'kb-search', 'zh-cn', '{"label":"知识库"}'::jsonb),
    ('home3-knowledge', 'kb-import', 'zh-cn', '{"label":"上传文件"}'::jsonb),
    ('home3-knowledge', 'kb-doc-wiki', 'zh-cn', '{"label":"知识管理"}'::jsonb),
    ('home3-knowledge', 'kb-chunks', 'zh-cn', '{"label":"文档处理"}'::jsonb),
    ('home3-knowledge', 'kb-llm-wiki', 'zh-cn', '{"label":"知识百科"}'::jsonb),
    ('home3-knowledge', 'kb-llm-wiki-v3', 'zh-cn', '{"label":"知识百科"}'::jsonb),
    ('home3-knowledge', 'kb-input-details', 'zh-cn', '{"label":"文档信息"}'::jsonb),
    ('home3-knowledge', 'kb-doc-structure', 'zh-cn', '{"label":"文档解析"}'::jsonb),
    ('home3-knowledge', 'kb-summary-tree', 'zh-cn', '{"label":"文档摘要树"}'::jsonb),
    ('home3-knowledge', 'kb-summary-graph', 'zh-cn', '{}'::jsonb),
    ('home3-knowledge', 'kb-semantic-projections', 'zh-cn', '{"label":"文档语义树"}'::jsonb),
    ('home3-knowledge', 'kb-topic-tree', 'zh-cn', '{}'::jsonb),
    ('home3-knowledge', 'kb-metrics', 'zh-cn', '{"label":"指标"}'::jsonb),
    ('home3-knowledge', 'kb-scene-blocks', 'zh-cn', '{"label":"场景"}'::jsonb),
    ('home3-knowledge', 'kb-products', 'zh-cn', '{}'::jsonb),
    ('home3-knowledge', 'kb-provision-tree', 'zh-cn', '{"label":"条款(标准)"}'::jsonb),
    ('home3-knowledge', 'kb-inventory-items', 'zh-cn', '{"label":"产品/零部件"}'::jsonb),
    ('home3-knowledge', 'kb-object-manager', 'zh-cn', '{"label":"本体关系图"}'::jsonb),
    ('home3-knowledge', 'kb-references', 'zh-cn', '{}'::jsonb),
    ('home3-knowledge', 'kb-formulas', 'zh-cn', '{}'::jsonb),
    ('home3-knowledge', 'kb-tables', 'zh-cn', '{}'::jsonb),
    ('home3-knowledge', 'kb-quotations', 'zh-cn', '{}'::jsonb),
    ('home3-knowledge', 'kb-case-studies', 'zh-cn', '{}'::jsonb),
    ('home3-knowledge', 'kb-workflow', 'zh-cn', '{}'::jsonb),
    ('home3-knowledge', 'kb-product-parts', 'zh-cn', '{}'::jsonb),
    ('home3-knowledge', 'kb-category-review', 'zh-cn', '{}'::jsonb)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- ── semos-workspace: en rows (empty content) ──
INSERT INTO kb.page_config (page_key, entry_key, language, content) VALUES
    ('semos-workspace', 'ws-kicker', 'en', '{}'::jsonb),
    ('semos-workspace', 'ws-banner-title', 'en', '{}'::jsonb),
    ('semos-workspace', 'ws-banner-subtitle', 'en', '{}'::jsonb),
    ('semos-workspace', 'ws-announcements', 'en', '{}'::jsonb),
    ('semos-workspace', 'knowledge_base', 'en', '{}'::jsonb),
    ('semos-workspace', 'chat', 'en', '{}'::jsonb),
    ('semos-workspace', 'search', 'en', '{}'::jsonb),
    ('semos-workspace', 'doc_reviews', 'en', '{}'::jsonb),
    ('semos-workspace', 'workflows', 'en', '{}'::jsonb),
    ('semos-workspace', 'agents', 'en', '{}'::jsonb)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- ── semos-workspace: zh-cn rows (overrides from workspace labels-zh-cn.toml) ──
INSERT INTO kb.page_config (page_key, entry_key, language, content) VALUES
    ('semos-workspace', 'ws-kicker', 'zh-cn', '{"label":"工作台"}'::jsonb),
    ('semos-workspace', 'ws-banner-title', 'zh-cn', '{"label":"我的工作台"}'::jsonb),
    ('semos-workspace', 'ws-banner-subtitle', 'zh-cn', '{"label":"管理你的知识库所需的一切。"}'::jsonb),
    ('semos-workspace', 'ws-announcements', 'zh-cn', '{}'::jsonb),
    ('semos-workspace', 'knowledge_base', 'zh-cn', '{"label":"知识库","description":"浏览和管理文档与知识工件。"}'::jsonb),
    ('semos-workspace', 'chat', 'zh-cn', '{"label":"对话知识库","description":"基于文档的可溯源问答。"}'::jsonb),
    ('semos-workspace', 'search', 'zh-cn', '{"label":"检索知识库","description":"关键词与语义混合检索。"}'::jsonb),
    ('semos-workspace', 'doc_reviews', 'zh-cn', '{"label":"文档审核","description":"运行并查看带引用依据的文档审核。"}'::jsonb),
    ('semos-workspace', 'workflows', 'zh-cn', '{"label":"工作流","description":"自动化多步骤文档处理流程。"}'::jsonb),
    ('semos-workspace', 'agents', 'zh-cn', '{"label":"智能体与运行平台","description":"基于知识构建并运行智能体。"}'::jsonb)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- Grant the current [system].access_roles set to every seeded entry so all
-- existing users (who have been given these roles) retain access. Only fills
-- rows that have no access_role yet, so admin edits are preserved on re-run.
UPDATE kb.page_config
   SET access_role = '["admin","root","guest","dev","k_engineer","trial"]'::jsonb
 WHERE page_key IN ('home3-knowledge', 'semos-workspace')
   AND access_role IS NULL;

-- +goose Down
DELETE FROM kb.page_config WHERE page_key IN ('home3-knowledge', 'semos-workspace');
DELETE FROM kb.page_def    WHERE page_key IN ('home3-knowledge', 'semos-workspace');

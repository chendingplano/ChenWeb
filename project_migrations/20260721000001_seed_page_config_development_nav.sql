-- +goose Up
-- Onboard the /development page (the Dashboard app-shell's NavRail sidebar menu)
-- onto the DB-backed page-config capability (KnowledgeStore spec 2026072001 §11;
-- ADR 2026072003). page_key = 'development'; entry_key = the id each nav node
-- already owns in web/src/lib/components/home3/nav-rail.svelte (top-level items,
-- children, and grandchildren). The pair (page_key, entry_key) is the durable
-- identity — never display text.
--
-- Content model matches the existing seed (20260720000002): the "en" rows carry
-- empty content ({}) so English falls back to the NavRail's hardcoded default
-- labels; the "zh-cn" rows carry Chinese translations. Default language is
-- [languages].default = "zh-cn", so the zh-cn row is authoritative for
-- accessible / enabled / access_role (set by the UPDATE at the end). Seeding all
-- current nodes keeps rendering identical (overlay model, spec §9.4) while
-- putting access control + translations in place from the start.

INSERT INTO kb.page_def (page_key, route, title, description) VALUES
    ('development', '/development', 'Development Workspace',
        'Configurable NavRail sidebar menu on /development (the Dashboard app shell).')
ON CONFLICT (page_key) DO NOTHING;

-- ── development: en rows (empty content = hardcoded English default) ──
INSERT INTO kb.page_config (page_key, entry_key, language, content) VALUES
    -- top-level items
    ('development', 'dashboard', 'en', '{}'::jsonb),
    ('development', 'chat', 'en', '{}'::jsonb),
    ('development', 'agents', 'en', '{}'::jsonb),
    ('development', 'skills', 'en', '{}'::jsonb),
    ('development', 'applications', 'en', '{}'::jsonb),
    ('development', 'coding', 'en', '{}'::jsonb),
    ('development', 'personal', 'en', '{}'::jsonb),
    ('development', 'knowledge', 'en', '{}'::jsonb),
    ('development', 'knowledge-engineering', 'en', '{}'::jsonb),
    ('development', 'tools', 'en', '{}'::jsonb),
    ('development', 'agent-platform', 'en', '{}'::jsonb),
    ('development', 'system-admin', 'en', '{}'::jsonb),
    ('development', 'my-workspace', 'en', '{}'::jsonb),
    ('development', 'settings', 'en', '{}'::jsonb),
    ('development', 'about', 'en', '{}'::jsonb),
    -- children / grandchildren
    ('development', 'doc-processor-dashboard', 'en', '{}'::jsonb),
    ('development', 'llm-activities', 'en', '{}'::jsonb),
    ('development', 'agents-my', 'en', '{}'::jsonb),
    ('development', 'agents-browse', 'en', '{}'::jsonb),
    ('development', 'agents-create', 'en', '{}'::jsonb),
    ('development', 'skills-all', 'en', '{}'::jsonb),
    ('development', 'skills-active', 'en', '{}'::jsonb),
    ('development', 'skills-create', 'en', '{}'::jsonb),
    ('development', 'apps-installed', 'en', '{}'::jsonb),
    ('development', 'apps-browse', 'en', '{}'::jsonb),
    ('development', 'apps-configure', 'en', '{}'::jsonb),
    ('development', 'apps-generate-doc', 'en', '{}'::jsonb),
    ('development', 'apps-document-review', 'en', '{}'::jsonb),
    ('development', 'coding-review', 'en', '{}'::jsonb),
    ('development', 'coding-gen', 'en', '{}'::jsonb),
    ('development', 'coding-debug', 'en', '{}'::jsonb),
    ('development', 'personal-tasks', 'en', '{}'::jsonb),
    ('development', 'personal-calendar', 'en', '{}'::jsonb),
    ('development', 'personal-email', 'en', '{}'::jsonb),
    ('development', 'ke-research-topics', 'en', '{}'::jsonb),
    ('development', 'kb-search-lab', 'en', '{}'::jsonb),
    ('development', 'flow', 'en', '{}'::jsonb),
    ('development', 'prompt-optimizer', 'en', '{}'::jsonb),
    ('development', 'openmetadata', 'en', '{}'::jsonb),
    ('development', 'ap-board', 'en', '{}'::jsonb),
    ('development', 'ap-agents', 'en', '{}'::jsonb),
    ('development', 'ap-projects', 'en', '{}'::jsonb),
    ('development', 'jetstream', 'en', '{}'::jsonb),
    ('development', 'sysadmin-jetstream-logs', 'en', '{}'::jsonb),
    ('development', 'sysadmin-jetstream-events', 'en', '{}'::jsonb),
    ('development', 'sysadmin-jetstream-subjects', 'en', '{}'::jsonb),
    ('development', 'sysadmin-logs', 'en', '{}'::jsonb),
    ('development', 'sysadmin-doc-proc-logs', 'en', '{}'::jsonb),
    ('development', 'sysadmin-llm-usage-logs', 'en', '{}'::jsonb),
    ('development', 'sysadmin-doc-review-logs', 'en', '{}'::jsonb),
    ('development', 'sysadmin-llm', 'en', '{}'::jsonb),
    ('development', 'sysadmin-llm-accounts', 'en', '{}'::jsonb),
    ('development', 'sysadmin-llm-model-profiles', 'en', '{}'::jsonb),
    ('development', 'sysadmin-llm-models', 'en', '{}'::jsonb),
    ('development', 'sysadmin-db', 'en', '{}'::jsonb),
    ('development', 'sysadmin-db-consistency', 'en', '{}'::jsonb),
    ('development', 'sysadmin-db-maint-log', 'en', '{}'::jsonb),
    ('development', 'sysadmin-db-resolve-ambiguous', 'en', '{}'::jsonb),
    ('development', 'sysadmin-users', 'en', '{}'::jsonb),
    ('development', 'sysadmin-user-management', 'en', '{}'::jsonb),
    ('development', 'sysadmin-role-management', 'en', '{}'::jsonb),
    ('development', 'sysadmin-access-controls', 'en', '{}'::jsonb),
    ('development', 'sysadmin-page-config', 'en', '{}'::jsonb),
    ('development', 'diary', 'en', '{}'::jsonb)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- ── development: zh-cn rows (Chinese label overrides) ──
INSERT INTO kb.page_config (page_key, entry_key, language, content) VALUES
    -- top-level items
    ('development', 'dashboard', 'zh-cn', '{"label":"仪表盘"}'::jsonb),
    ('development', 'chat', 'zh-cn', '{"label":"对话"}'::jsonb),
    ('development', 'agents', 'zh-cn', '{"label":"智能体"}'::jsonb),
    ('development', 'skills', 'zh-cn', '{"label":"技能"}'::jsonb),
    ('development', 'applications', 'zh-cn', '{"label":"应用"}'::jsonb),
    ('development', 'coding', 'zh-cn', '{"label":"编程助手"}'::jsonb),
    ('development', 'personal', 'zh-cn', '{"label":"个人助手"}'::jsonb),
    ('development', 'knowledge', 'zh-cn', '{"label":"知识系统"}'::jsonb),
    ('development', 'knowledge-engineering', 'zh-cn', '{"label":"知识工程"}'::jsonb),
    ('development', 'tools', 'zh-cn', '{"label":"工具"}'::jsonb),
    ('development', 'agent-platform', 'zh-cn', '{"label":"智能体平台"}'::jsonb),
    ('development', 'system-admin', 'zh-cn', '{"label":"系统管理"}'::jsonb),
    ('development', 'my-workspace', 'zh-cn', '{"label":"我的工作台"}'::jsonb),
    ('development', 'settings', 'zh-cn', '{"label":"设置"}'::jsonb),
    ('development', 'about', 'zh-cn', '{"label":"关于"}'::jsonb),
    -- children / grandchildren
    ('development', 'doc-processor-dashboard', 'zh-cn', '{"label":"文档处理器"}'::jsonb),
    ('development', 'llm-activities', 'zh-cn', '{"label":"LLM 活动"}'::jsonb),
    ('development', 'agents-my', 'zh-cn', '{"label":"我的智能体"}'::jsonb),
    ('development', 'agents-browse', 'zh-cn', '{"label":"浏览智能体库"}'::jsonb),
    ('development', 'agents-create', 'zh-cn', '{"label":"创建智能体"}'::jsonb),
    ('development', 'skills-all', 'zh-cn', '{"label":"全部技能"}'::jsonb),
    ('development', 'skills-active', 'zh-cn', '{"label":"已启用"}'::jsonb),
    ('development', 'skills-create', 'zh-cn', '{"label":"新建技能"}'::jsonb),
    ('development', 'apps-installed', 'zh-cn', '{"label":"已安装"}'::jsonb),
    ('development', 'apps-browse', 'zh-cn', '{"label":"浏览"}'::jsonb),
    ('development', 'apps-configure', 'zh-cn', '{"label":"配置"}'::jsonb),
    ('development', 'apps-generate-doc', 'zh-cn', '{"label":"生成文档"}'::jsonb),
    ('development', 'apps-document-review', 'zh-cn', '{"label":"文档审核"}'::jsonb),
    ('development', 'coding-review', 'zh-cn', '{"label":"代码审查"}'::jsonb),
    ('development', 'coding-gen', 'zh-cn', '{"label":"代码生成"}'::jsonb),
    ('development', 'coding-debug', 'zh-cn', '{"label":"调试器"}'::jsonb),
    ('development', 'personal-tasks', 'zh-cn', '{"label":"任务"}'::jsonb),
    ('development', 'personal-calendar', 'zh-cn', '{"label":"日历"}'::jsonb),
    ('development', 'personal-email', 'zh-cn', '{"label":"邮件"}'::jsonb),
    ('development', 'ke-research-topics', 'zh-cn', '{"label":"研究课题"}'::jsonb),
    ('development', 'kb-search-lab', 'zh-cn', '{"label":"知识库检索实验室"}'::jsonb),
    ('development', 'flow', 'zh-cn', '{"label":"流程"}'::jsonb),
    ('development', 'prompt-optimizer', 'zh-cn', '{"label":"提示词优化器"}'::jsonb),
    ('development', 'openmetadata', 'zh-cn', '{}'::jsonb),
    ('development', 'ap-board', 'zh-cn', '{"label":"看板"}'::jsonb),
    ('development', 'ap-agents', 'zh-cn', '{"label":"智能体"}'::jsonb),
    ('development', 'ap-projects', 'zh-cn', '{"label":"项目"}'::jsonb),
    ('development', 'jetstream', 'zh-cn', '{}'::jsonb),
    ('development', 'sysadmin-jetstream-logs', 'zh-cn', '{"label":"JetStream 日志"}'::jsonb),
    ('development', 'sysadmin-jetstream-events', 'zh-cn', '{"label":"JetStream 事件"}'::jsonb),
    ('development', 'sysadmin-jetstream-subjects', 'zh-cn', '{"label":"JetStream 主题"}'::jsonb),
    ('development', 'sysadmin-logs', 'zh-cn', '{"label":"日志"}'::jsonb),
    ('development', 'sysadmin-doc-proc-logs', 'zh-cn', '{"label":"文档处理器日志"}'::jsonb),
    ('development', 'sysadmin-llm-usage-logs', 'zh-cn', '{"label":"LLM 使用日志"}'::jsonb),
    ('development', 'sysadmin-doc-review-logs', 'zh-cn', '{"label":"文档审核日志"}'::jsonb),
    ('development', 'sysadmin-llm', 'zh-cn', '{}'::jsonb),
    ('development', 'sysadmin-llm-accounts', 'zh-cn', '{"label":"LLM 账户"}'::jsonb),
    ('development', 'sysadmin-llm-model-profiles', 'zh-cn', '{"label":"模型配置"}'::jsonb),
    ('development', 'sysadmin-llm-models', 'zh-cn', '{"label":"LLM 模型"}'::jsonb),
    ('development', 'sysadmin-db', 'zh-cn', '{"label":"数据库维护"}'::jsonb),
    ('development', 'sysadmin-db-consistency', 'zh-cn', '{"label":"一致性检查"}'::jsonb),
    ('development', 'sysadmin-db-maint-log', 'zh-cn', '{"label":"维护日志"}'::jsonb),
    ('development', 'sysadmin-db-resolve-ambiguous', 'zh-cn', '{"label":"解决歧义对象"}'::jsonb),
    ('development', 'sysadmin-users', 'zh-cn', '{"label":"用户与权限"}'::jsonb),
    ('development', 'sysadmin-user-management', 'zh-cn', '{"label":"用户管理"}'::jsonb),
    ('development', 'sysadmin-role-management', 'zh-cn', '{"label":"角色管理"}'::jsonb),
    ('development', 'sysadmin-access-controls', 'zh-cn', '{"label":"访问控制"}'::jsonb),
    ('development', 'sysadmin-page-config', 'zh-cn', '{"label":"页面内容"}'::jsonb),
    ('development', 'diary', 'zh-cn', '{"label":"日记"}'::jsonb)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- Grant the current [system].access_roles set to every seeded entry so all
-- existing users retain access. Only fills rows with no access_role yet, so
-- admin edits are preserved on re-run.
UPDATE kb.page_config
   SET access_role = '["admin","root","guest","dev","k_engineer","trial"]'::jsonb
 WHERE page_key = 'development'
   AND access_role IS NULL;

-- +goose Down
DELETE FROM kb.page_config WHERE page_key = 'development';
DELETE FROM kb.page_def    WHERE page_key = 'development';

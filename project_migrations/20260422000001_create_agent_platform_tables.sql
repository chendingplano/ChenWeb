-- +goose Up

-- =============================================================================
-- Agent Platform (home3) — core tables.
--
-- Scope: workspaces, agents, issues (kanban), comments, and task-run schema
-- stubs for milestone M1. Schema owned by ChenWeb (project-scope, not shared).
--
-- Conventions (match prompt_optimizer_* migration):
--   - IDs:          VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text
--   - user IDs:     VARCHAR(64)  (Kratos identity id)
--   - timestamps:   TIMESTAMPTZ NOT NULL DEFAULT NOW()
--   - every index/constraint uses IF NOT EXISTS semantics
--
-- Nothing in this migration derives from multica-ai/multica's schema; table
-- names are prefixed `ap_` (agent platform) to avoid any collision and to
-- make later cross-feature queries readable.
-- =============================================================================

-- -----------------------------------------------------------------------------
-- Workspaces: tenant boundary. One user may belong to many via ap_workspace_member.
-- The owner_user_id is the creator; ownership can later be reassigned by updating
-- the row and/or a membership record with role='owner'.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ap_workspace (
    id            VARCHAR(64)  PRIMARY KEY DEFAULT gen_random_uuid()::text,
    slug          TEXT         NOT NULL,
    name          TEXT         NOT NULL,
    owner_user_id VARCHAR(64)  NOT NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS ap_workspace_slug_uidx
    ON ap_workspace(LOWER(slug));
CREATE INDEX IF NOT EXISTS ap_workspace_owner_idx
    ON ap_workspace(owner_user_id);

-- -----------------------------------------------------------------------------
-- Workspace membership: user ↔ workspace with role.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ap_workspace_member (
    workspace_id VARCHAR(64)  NOT NULL
        REFERENCES ap_workspace(id) ON DELETE CASCADE,
    user_id      VARCHAR(64)  NOT NULL,
    role         TEXT         NOT NULL
        CHECK (role IN ('owner','member','viewer')),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (workspace_id, user_id)
);
CREATE INDEX IF NOT EXISTS ap_workspace_member_user_idx
    ON ap_workspace_member(user_id);

-- -----------------------------------------------------------------------------
-- Per-workspace counter for sequential issue numbers (§9.4). One row per
-- workspace; allocated inside a transaction with SELECT ... FOR UPDATE to
-- guarantee uniqueness under concurrency.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ap_workspace_counter (
    workspace_id      VARCHAR(64)  PRIMARY KEY
        REFERENCES ap_workspace(id) ON DELETE CASCADE,
    next_issue_number INT          NOT NULL DEFAULT 1
);

-- -----------------------------------------------------------------------------
-- Projects: optional grouping for issues (Linear-style).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ap_project (
    id           VARCHAR(64)  PRIMARY KEY DEFAULT gen_random_uuid()::text,
    workspace_id VARCHAR(64)  NOT NULL
        REFERENCES ap_workspace(id) ON DELETE CASCADE,
    name         TEXT         NOT NULL,
    description  TEXT         NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS ap_project_workspace_idx
    ON ap_project(workspace_id);

-- -----------------------------------------------------------------------------
-- Agents: named agent profiles (e.g. "Claude-Backend"). runtime_kind selects
-- which Runner implementation executes their tasks. instructions is the
-- per-agent system prompt appended to each task.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ap_agent (
    id            VARCHAR(64)  PRIMARY KEY DEFAULT gen_random_uuid()::text,
    workspace_id  VARCHAR(64)  NOT NULL
        REFERENCES ap_workspace(id) ON DELETE CASCADE,
    name          TEXT         NOT NULL,
    avatar_emoji  TEXT         NOT NULL DEFAULT '🤖',
    runtime_kind  TEXT         NOT NULL
        CHECK (runtime_kind IN ('claude_code','codex','openclaw','opencode')),
    model         TEXT         NOT NULL DEFAULT '',
    instructions  TEXT         NOT NULL DEFAULT '',
    enabled       BOOLEAN      NOT NULL DEFAULT TRUE,
    archived_at   TIMESTAMPTZ  NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS ap_agent_workspace_name_uidx
    ON ap_agent(workspace_id, LOWER(name))
    WHERE archived_at IS NULL;

-- -----------------------------------------------------------------------------
-- Issues: kanban cards. issue_number is monotonic per workspace (allocated
-- from ap_workspace_counter). At most one assignee (human OR agent).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ap_issue (
    id                VARCHAR(64)  PRIMARY KEY DEFAULT gen_random_uuid()::text,
    workspace_id      VARCHAR(64)  NOT NULL
        REFERENCES ap_workspace(id) ON DELETE CASCADE,
    project_id        VARCHAR(64)  NULL
        REFERENCES ap_project(id) ON DELETE SET NULL,
    issue_number      INT          NOT NULL,
    title             TEXT         NOT NULL,
    description       TEXT         NOT NULL DEFAULT '',
    status            TEXT         NOT NULL DEFAULT 'backlog'
        CHECK (status IN ('backlog','todo','in_progress','in_review','done','canceled')),
    priority          SMALLINT     NOT NULL DEFAULT 0
        CHECK (priority BETWEEN 0 AND 4),        -- 0=none,1=low,2=medium,3=high,4=urgent
    board_order       DOUBLE PRECISION NOT NULL DEFAULT 0,  -- fractional rank within a column
    assignee_user_id  VARCHAR(64)  NULL,
    assignee_agent_id VARCHAR(64)  NULL
        REFERENCES ap_agent(id) ON DELETE SET NULL,
    created_by        VARCHAR(64)  NOT NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT ap_issue_single_assignee
        CHECK (assignee_user_id IS NULL OR assignee_agent_id IS NULL)
);
CREATE UNIQUE INDEX IF NOT EXISTS ap_issue_workspace_number_uidx
    ON ap_issue(workspace_id, issue_number);
CREATE INDEX IF NOT EXISTS ap_issue_status_idx
    ON ap_issue(workspace_id, status);
CREATE INDEX IF NOT EXISTS ap_issue_project_idx
    ON ap_issue(project_id);
CREATE INDEX IF NOT EXISTS ap_issue_assignee_user_idx
    ON ap_issue(assignee_user_id)
    WHERE assignee_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS ap_issue_assignee_agent_idx
    ON ap_issue(assignee_agent_id)
    WHERE assignee_agent_id IS NOT NULL;

-- -----------------------------------------------------------------------------
-- Comments: one thread per issue. Author is exactly one of user/agent.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ap_comment (
    id              VARCHAR(64)  PRIMARY KEY DEFAULT gen_random_uuid()::text,
    issue_id        VARCHAR(64)  NOT NULL
        REFERENCES ap_issue(id) ON DELETE CASCADE,
    author_user_id  VARCHAR(64)  NULL,
    author_agent_id VARCHAR(64)  NULL
        REFERENCES ap_agent(id) ON DELETE SET NULL,
    body            TEXT         NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT ap_comment_single_author
        CHECK ((author_user_id IS NULL) <> (author_agent_id IS NULL))
);
CREATE INDEX IF NOT EXISTS ap_comment_issue_idx
    ON ap_comment(issue_id, created_at);

-- -----------------------------------------------------------------------------
-- Task runs (schema stub for M1). One execution attempt of an issue by an
-- agent. M0 creates no rows here; M1 populates via the worker pool.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ap_task_run (
    id                VARCHAR(64)  PRIMARY KEY DEFAULT gen_random_uuid()::text,
    workspace_id      VARCHAR(64)  NOT NULL
        REFERENCES ap_workspace(id) ON DELETE CASCADE,
    issue_id          VARCHAR(64)  NOT NULL
        REFERENCES ap_issue(id) ON DELETE CASCADE,
    agent_id          VARCHAR(64)  NOT NULL
        REFERENCES ap_agent(id) ON DELETE RESTRICT,
    status            TEXT         NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued','claimed','running','succeeded','failed','canceled')),
    queued_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    claimed_at        TIMESTAMPTZ  NULL,
    started_at        TIMESTAMPTZ  NULL,
    finished_at       TIMESTAMPTZ  NULL,
    exit_code         INT          NULL,
    error_message     TEXT         NULL,
    runner_version    TEXT         NULL,
    workdir_path      TEXT         NULL,
    lease_expires_at  TIMESTAMPTZ  NULL
);
CREATE INDEX IF NOT EXISTS ap_task_run_status_idx
    ON ap_task_run(status, queued_at);
CREATE INDEX IF NOT EXISTS ap_task_run_issue_idx
    ON ap_task_run(issue_id, queued_at DESC);

-- -----------------------------------------------------------------------------
-- Task events (schema stub for M1). Append-only event log for each task run.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ap_task_event (
    id          BIGSERIAL    PRIMARY KEY,
    task_run_id VARCHAR(64)  NOT NULL
        REFERENCES ap_task_run(id) ON DELETE CASCADE,
    kind        TEXT         NOT NULL
        CHECK (kind IN ('stdout','stderr','status','heartbeat','artifact','error')),
    payload     TEXT         NOT NULL DEFAULT '',
    at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS ap_task_event_run_idx
    ON ap_task_event(task_run_id, id);

-- -----------------------------------------------------------------------------
-- Artifacts (schema stub for M1). Files produced by a run (diffs, logs, etc.).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ap_artifact (
    id          VARCHAR(64)  PRIMARY KEY DEFAULT gen_random_uuid()::text,
    task_run_id VARCHAR(64)  NOT NULL
        REFERENCES ap_task_run(id) ON DELETE CASCADE,
    path        TEXT         NOT NULL,
    kind        TEXT         NOT NULL
        CHECK (kind IN ('diff','log','file','other')),
    size_bytes  BIGINT       NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS ap_artifact_run_idx
    ON ap_artifact(task_run_id, created_at);


-- +goose Down

DROP TABLE IF EXISTS ap_artifact;
DROP TABLE IF EXISTS ap_task_event;
DROP TABLE IF EXISTS ap_task_run;
DROP TABLE IF EXISTS ap_comment;
DROP TABLE IF EXISTS ap_issue;
DROP TABLE IF EXISTS ap_agent;
DROP TABLE IF EXISTS ap_project;
DROP TABLE IF EXISTS ap_workspace_counter;
DROP TABLE IF EXISTS ap_workspace_member;
DROP TABLE IF EXISTS ap_workspace;

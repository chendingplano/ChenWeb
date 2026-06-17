-- +goose Up
-- Stage 1 of "drop persistence, compute connected_artifacts on demand" (Option A; see
-- KnowledgeStore/Capsules/coding-capsules/llm-wiki/artifact-connections.md §2.2).
--
-- This migration is additive and reversible: it adds the queryable chunk-range table and
-- the on-demand function. It does NOT yet drop kb.<family>.connected_artifacts or reroute
-- readers — that is Stage 2, after parity is confirmed on a reprocessed record.

-- Per-chunk line ranges, keyed by the positional chunk_id ('<record>_chk_<index>') used in
-- connected_artifacts.chunks. Populated from the canonical fixed-size chunks in Phase C.
-- source_line_spans is the source of truth; line_range is derived (and GiST-indexed) so the
-- on-demand function can intersect chunks with artifacts via the `&&` operator.
CREATE TABLE IF NOT EXISTS kb.chunk_ranges (
    input_record_id   int8        NOT NULL,
    chunk_id          text        NOT NULL,
    source_line_spans jsonb       NOT NULL DEFAULT '[]'::jsonb,
    line_range        int8multirange GENERATED ALWAYS AS (kb.line_spans_to_int8multirange(source_line_spans)) STORED,
    create_time       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT kb_chunk_ranges_pkey PRIMARY KEY (input_record_id, chunk_id)
);
CREATE INDEX IF NOT EXISTS idx_kb_chunk_ranges_line_range
    ON kb.chunk_ranges USING gist (line_range);

-- On-demand connected_artifacts for one artifact. Returns the same JSON shape that was
-- previously materialized: a "chunks" array plus one array per artifact family except the
-- source family itself, each present even when empty.
--
-- The caller passes the source artifact's own line_range (every family base table has the
-- generated line_range column), not an id. This avoids the id-mapping problem: a family's
-- base-table id (e.g. kb.summaries.summary_id = '..._sum_0_0005') is not always the registry
-- artifact_id (e.g. '..._sum_1'), so the caller passes the base-table PK (source_row_id),
-- which kb.search_artifacts records for every artifact. The whole graph is in registry-id
-- space: connected_artifacts already references targets by registry artifact_id, so self is
-- resolved through the registry too (via source_row_id) to keep the graph symmetric. Self is
-- excluded by omitting the whole source family, so the source artifact's own id is not
-- emitted. A base artifact not present in the registry yields an empty result.
--
-- Summaries are special: only LEVEL-0 summaries participate in line-overlap connections,
-- and the search registry is unreliable for summary levels (it can omit some level-0
-- summaries and include level-1/2 ones). So summary edges are sourced from kb.summaries
-- WHERE summary_level = 0 (authoritative), referenced by base summary_id, both when a
-- summary is the source and when it is a target. A level-1/2 summary as source yields an
-- empty result.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.connected_artifacts(p_record_id int8, p_self_type text, p_source_row_id int8)
RETURNS jsonb
LANGUAGE sql
STABLE
AS $fn$
    WITH self AS (
        SELECT line_range FROM kb.summaries
          WHERE p_self_type = 'summary' AND input_record_id = p_record_id
            AND id = p_source_row_id AND summary_level = 0
        UNION ALL
        SELECT line_range FROM kb.search_artifacts
          WHERE p_self_type <> 'summary' AND input_record_id = p_record_id
            AND artifact_type = p_self_type AND source_row_id = p_source_row_id
    ),
    reg AS (  -- non-summary registry targets (exclude self family and summary)
        SELECT t.artifact_type, t.artifact_id
        FROM kb.search_artifacts t, self s
        WHERE t.input_record_id = p_record_id
          AND t.artifact_type NOT IN (p_self_type, 'summary')
          AND s.line_range IS NOT NULL
          AND t.line_range && s.line_range
    ),
    summ AS (  -- level-0 summary targets from kb.summaries (skip when self is a summary)
        SELECT 'summary'::text AS artifact_type, su.summary_id AS artifact_id
        FROM kb.summaries su, self s
        WHERE p_self_type <> 'summary' AND su.input_record_id = p_record_id
          AND su.summary_level = 0
          AND s.line_range IS NOT NULL
          AND su.line_range && s.line_range
    ),
    arts AS (
        SELECT * FROM reg
        UNION ALL
        SELECT * FROM summ
    ),
    chunk_hits AS (
        SELECT c.chunk_id, c.line_range
        FROM kb.chunk_ranges c, self s
        WHERE c.input_record_id = p_record_id
          AND s.line_range IS NOT NULL
          AND c.line_range && s.line_range
    ),
    fams(reg_type, json_key) AS (
        VALUES ('semantic_projection','semantic_projects'),
               ('summary','summaries'),
               ('topic','topics'),
               ('scene_block','scenes'),
               ('provision','provisions'),
               ('entity','entities'),
               ('relation','relations'),
               ('metric','metrics'),
               ('inventory_item','inv_items')
    )
    SELECT jsonb_build_object(
               'chunks',
               COALESCE((SELECT jsonb_agg(chunk_id ORDER BY line_range, chunk_id) FROM chunk_hits), '[]'::jsonb)
           )
           || COALESCE((
                  SELECT jsonb_object_agg(json_key, ids)
                  FROM (
                      SELECT f.json_key,
                             COALESCE(jsonb_agg(a.artifact_id ORDER BY a.artifact_id)
                                      FILTER (WHERE a.artifact_id IS NOT NULL), '[]'::jsonb) AS ids
                      FROM fams f
                      LEFT JOIN arts a ON a.artifact_type = f.reg_type
                      WHERE f.reg_type <> p_self_type
                      GROUP BY f.json_key
                  ) per_family
              ), '{}'::jsonb);
$fn$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS kb.connected_artifacts(int8, text, int8);
DROP INDEX IF EXISTS kb.idx_kb_chunk_ranges_line_range;
DROP TABLE IF EXISTS kb.chunk_ranges;

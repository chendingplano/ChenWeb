-- +goose Up
-- The term identity foundation mirrors INSERTs, while release tagging updates
-- the legacy term row after the snapshot is created. Backfill the mirrored
-- revision state for releases created before release tagging synchronized both
-- representations.
UPDATE kb.ontology_term_revisions AS revision
SET status = term.status,
    released_in_release_id = term.released_in_release_id,
    modify_time = term.modify_time,
    modify_by = term.modify_by
FROM kb.ontology_terms AS term
WHERE term.id = revision.source_term_row_id
  AND (revision.status, revision.released_in_release_id, revision.modify_time, revision.modify_by)
      IS DISTINCT FROM (term.status, term.released_in_release_id, term.modify_time, term.modify_by);

-- +goose Down
-- The synchronization is a corrective data migration and is intentionally not
-- reversed; reverting it would recreate the split state this migration fixes.

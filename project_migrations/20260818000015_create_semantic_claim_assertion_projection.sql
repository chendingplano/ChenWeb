-- +goose Up
-- ADR 2026081701 DR5 compatibility surface: only assertions whose stored
-- logical identity is the registry claim ID appear here. Legacy occurrence
-- keys remain readable from kb.semantic_assertions but are not misrepresented
-- as canonical claims.

CREATE OR REPLACE VIEW kb.semantic_claim_assertion_projection AS
SELECT
    assertion.id AS assertion_id,
    assertion.logical_identity_key,
    assertion.revision AS assertion_revision,
    assertion.instance_of_term_id,
    claim.claim_id,
    claim.key_version,
    claim.class_term_id,
    claim.canonical_key,
    claim.identity_payload
FROM kb.semantic_assertions AS assertion
JOIN kb.semantic_claim_identities AS claim
    ON claim.claim_id = assertion.logical_identity_key
WHERE assertion.logical_identity_key = claim.claim_id;

-- +goose Down
DROP VIEW IF EXISTS kb.semantic_claim_assertion_projection;

package docprocessing

import (
	"fmt"
	"strings"
)

// This file implements the standalone, line-anchored endpoint link step from
// ADR 2026061702 (DR3), which supersedes the entity-aware resolveAndLinkRelations
// of ADR 2026061302. Relations are now extracted free-form (prompt-extract-
// relations-v2.md) and carry their own subject_lines/predicate_lines/object_lines.
// linkRelationEndpoints runs AFTER both extraction passes complete (guarded by
// EntitiesExist && RelationsExist, DR4) and resolves each endpoint to an entity:
//
//   1. name match    — normalized surface form in the entity index (precision);
//   2. positional     — unambiguous line-span overlap when the name differs
//                       (the line-anchor recall win; only when exactly one entity
//                       overlaps, so we never guess between several);
//   3. provisional    — mint, seeding the new entity with the endpoint's own line
//                       spans, and feed it to scheduled reconciliation (2026061701).
//
// Pure (no DB / no LLM) so it is unit-testable.

// spansOverlapAny reports whether any of an entity's line spans intersects any of
// the endpoint's line spans (inclusive ranges).
func spansOverlapAny(entitySpans, endpointLines []string) bool {
	for _, es := range entitySpans {
		s1, e1 := parseLineSpanRange(es)
		if s1 <= 0 {
			continue
		}
		for _, el := range endpointLines {
			s2, e2 := parseLineSpanRange(el)
			if s2 <= 0 {
				continue
			}
			if s1 <= e2 && s2 <= e1 {
				return true
			}
		}
	}
	return false
}

// linkRelationEndpoints links each free-form relation's subject/object to an
// entity_id, minting provisional entities for the unresolved ones. Returns the
// linked + deduplicated relations and the (possibly extended) entity slice.
func linkRelationEndpoints(
	relations []map[string]any,
	entities []map[string]any,
	recordID int64,
	nextEntityIndex int,
	createTime string,
) ([]map[string]any, []map[string]any) {
	nameIdx := buildEntityResolutionIndex(entities)
	idToEntity := make(map[string]map[string]any, len(entities))
	for _, e := range entities {
		if id := strings.TrimSpace(asString(e["entity_id"])); id != "" {
			idToEntity[id] = e
		}
	}

	// matchByName resolves a surface form against the entity index, with the
	// "(category)" suffix retry the LLM occasionally introduces.
	matchByName := func(surface string) (string, bool) {
		f := normalizeSurfaceForm(surface)
		if f == "" {
			return "", false
		}
		if id, ok := nameIdx[f]; ok {
			return id, true
		}
		if stripped, ok := stripCategorySuffix(strings.TrimSpace(surface)); ok {
			if fs := normalizeSurfaceForm(stripped); fs != "" {
				if id, ok2 := nameIdx[fs]; ok2 {
					return id, true
				}
			}
		}
		return "", false
	}

	// matchByPosition returns the entity_id when exactly one entity overlaps the
	// endpoint's lines; ambiguous (>1) or empty overlaps return false so we never
	// guess between candidates.
	matchByPosition := func(endpointLines []string) (string, bool) {
		if len(endpointLines) == 0 {
			return "", false
		}
		var hitID string
		hits := 0
		for _, e := range entities {
			if spansOverlapAny(toStringSlice(e["line_spans"]), endpointLines) {
				hits++
				hitID = strings.TrimSpace(asString(e["entity_id"]))
			}
		}
		if hits == 1 {
			return hitID, true
		}
		return "", false
	}

	// resolve links one endpoint, minting a provisional (seeded with its own line
	// spans) when neither name nor unambiguous position resolves it.
	resolve := func(surface, desc string, endpointLines []string) string {
		if id, ok := matchByName(surface); ok {
			return id
		}
		if id, ok := matchByPosition(endpointLines); ok {
			return id
		}
		id := fmt.Sprintf("%d_ent_%d", recordID, nextEntityIndex)
		nextEntityIndex++
		prov := map[string]any{
			"entity_id":         id,
			"entity":            strings.TrimSpace(surface),
			"entity_en":         "",
			"entity_type":       "",
			"entity_type_en":    "",
			"aliases":           []string{},
			"aliases_en":        []string{},
			"desc":              strings.TrimSpace(desc),
			"desc_en":           "",
			"keywords":          []string{},
			"keywords_en":       []string{},
			"line_spans":        sortedUniqueSpans(endpointLines),
			"entity_categories": []string{},
			"confidence":        0.0,
			"chunk_seq_no":      0,
			"entity_status":     entityStatusProvisional,
			"create_time":       createTime,
		}
		entities = append(entities, prov)
		idToEntity[id] = prov
		if f := normalizeSurfaceForm(surface); f != "" {
			if _, exists := nameIdx[f]; !exists {
				nameIdx[f] = id
			}
		}
		return id
	}

	deduped := make([]map[string]any, 0, len(relations))
	seen := make(map[string]struct{})
	for _, r := range relations {
		subjLines := toStringSlice(r["subject_lines"])
		objLines := toStringSlice(r["object_lines"])
		predLines := toStringSlice(r["predicate_lines"])

		subjID := resolve(asString(r["subject"]), asString(r["subject_desc"]), subjLines)
		objID := resolve(asString(r["object"]), asString(r["object_desc"]), objLines)
		r["subject_entity_id"] = subjID
		r["object_entity_id"] = objID

		// Relation grounding comes from the prompt's own line citations (DR3 — a
		// strict improvement over endpoint-derived spans). Fall back to the union
		// of endpoint spans only when the prompt cited no lines.
		promptSpans := sortedUniqueSpans(append(append(append([]string{}, subjLines...), predLines...), objLines...))
		if len(promptSpans) > 0 {
			r["line_spans"] = promptSpans
		} else if derived := relationSpansFromEndpoints(idToEntity[subjID], idToEntity[objID]); len(derived) > 0 {
			r["line_spans"] = derived
		}

		key := subjID + "\x00" + normalizePredicate(asString(r["predicate"])) + "\x00" + objID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, r)
	}

	// Provisionals are already seeded with their own endpoint's line spans (DR3).
	// Only those whose endpoint cited no lines remain empty; ground just those
	// from their referencing relations, without broadening the precisely-seeded ones.
	backfillEmptyProvisionalSpans(deduped, idToEntity)

	return deduped, entities
}

// backfillEmptyProvisionalSpans grounds provisional entities that still have no
// line spans (their endpoint cited none) at the union of their referencing
// relations' spans. Unlike backfillProvisionalEntitySpans it does NOT broaden a
// provisional already seeded with its own endpoint spans, preserving the precise
// per-mention grounding from DR3.
func backfillEmptyProvisionalSpans(relations []map[string]any, idToEntity map[string]map[string]any) {
	for _, r := range relations {
		spans := toStringSlice(r["line_spans"])
		if len(spans) == 0 {
			continue
		}
		for _, idKey := range []string{"subject_entity_id", "object_entity_id"} {
			e := idToEntity[strings.TrimSpace(asString(r[idKey]))]
			if e == nil || asString(e["entity_status"]) != entityStatusProvisional {
				continue
			}
			if len(toStringSlice(e["line_spans"])) > 0 {
				continue
			}
			e["line_spans"] = sortedUniqueSpans(spans)
		}
	}
}

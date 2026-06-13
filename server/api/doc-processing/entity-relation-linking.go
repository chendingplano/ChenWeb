package docprocessing

import (
	"fmt"
	"sort"
	"strings"
)

// This file implements the entity-linked relation design from ADR 2026061302:
//
//   - Phase 1.5 (D6): consolidateEntities merges duplicate entities extracted
//     per chunk into one canonical entity (and one canonical entity_id).
//   - D5: buildRelationWindows partitions consolidated entities into overlapping
//     positional windows so relation extraction is local and bounded.
//   - D4: resolveAndLinkRelations resolves each relation endpoint to a canonical
//     entity_id, minting a provisional entity when no match exists, then
//     deduplicates the resulting edges.
//
// All functions here are pure (no DB / no LLM) so they are unit-testable.

// normalizeSurfaceForm folds a surface string to a comparison key:
// trimmed, lower-cased, internal whitespace collapsed to single spaces.
func normalizeSurfaceForm(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// entitySurfaceForms returns every normalized surface form by which an entity
// may be referenced: its name, English name, and all aliases (both languages).
func entitySurfaceForms(e map[string]any) []string {
	var forms []string
	add := func(raw string) {
		if f := normalizeSurfaceForm(raw); f != "" {
			forms = append(forms, f)
		}
	}
	add(asString(e["entity"]))
	add(asString(e["entity_en"]))
	for _, a := range toStringSlice(e["aliases"]) {
		add(a)
	}
	for _, a := range toStringSlice(e["aliases_en"]) {
		add(a)
	}
	return forms
}

// ---- Phase 1.5: consolidation (D6) ----

// consolidateEntities merges entities that share any surface form (name or
// alias, either language) into a single canonical entity. It does NOT assign
// entity_id; callers assign ids after consolidation so duplicates collapse to
// one id. Input order is preserved by canonical first-appearance.
func consolidateEntities(entities []map[string]any) []map[string]any {
	if len(entities) <= 1 {
		return entities
	}

	parent := make([]int, len(entities))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		// Keep the earlier index as root to preserve first-appearance order.
		if ra < rb {
			parent[rb] = ra
		} else {
			parent[ra] = rb
		}
	}

	formOwner := make(map[string]int)
	for i, e := range entities {
		for _, f := range entitySurfaceForms(e) {
			if owner, ok := formOwner[f]; ok {
				union(i, owner)
			} else {
				formOwner[f] = i
			}
		}
	}

	// Group member indices by root, preserving first-appearance of each group.
	groups := make(map[int][]int)
	var rootOrder []int
	for i := range entities {
		r := find(i)
		if _, ok := groups[r]; !ok {
			rootOrder = append(rootOrder, r)
		}
		groups[r] = append(groups[r], i)
	}

	out := make([]map[string]any, 0, len(rootOrder))
	for _, r := range rootOrder {
		out = append(out, mergeEntityGroup(entities, groups[r]))
	}
	return out
}

// mergeEntityGroup folds a group of duplicate entities into one. The canonical
// name is the fullest (longest) member name so an abbreviation never wins over
// its expansion; the highest-confidence member anchors the remaining scalar
// attributes. Other members fill blanks and contribute
// aliases/keywords/spans/categories.
func mergeEntityGroup(entities []map[string]any, members []int) map[string]any {
	// Attribute anchor = highest-confidence member, ties broken by earliest index.
	anchor := members[0]
	for _, idx := range members[1:] {
		if toFloat(entities[idx]["confidence"]) > toFloat(entities[anchor]["confidence"]) {
			anchor = idx
		}
	}
	// Name anchor = longest member name, ties broken by higher confidence.
	canonicalName := pickCanonicalName(entities, members, "entity")
	canonicalNameEn := pickCanonicalName(entities, members, "entity_en")

	merged := map[string]any{
		"entity":         canonicalName,
		"entity_en":      canonicalNameEn,
		"entity_type":    strings.TrimSpace(asString(entities[anchor]["entity_type"])),
		"entity_type_en": strings.TrimSpace(asString(entities[anchor]["entity_type_en"])),
		"desc":           strings.TrimSpace(asString(entities[anchor]["desc"])),
		"desc_en":        strings.TrimSpace(asString(entities[anchor]["desc_en"])),
		"entity_status":  entityStatusExtracted,
	}
	canonicalNameKey := normalizeSurfaceForm(canonicalName)
	canonicalNameEnKey := normalizeSurfaceForm(canonicalNameEn)

	var aliases, aliasesEn, keywords, keywordsEn, spans, categories []string
	maxConf := 0.0
	minChunk := -1
	for _, idx := range members {
		e := entities[idx]
		// Fill blank scalars from any member.
		fillBlank(merged, "entity_en", e)
		fillBlank(merged, "entity_type", e)
		fillBlank(merged, "entity_type_en", e)
		fillBlank(merged, "desc", e)
		fillBlank(merged, "desc_en", e)

		// Other members' names become aliases of the canonical name.
		if n := strings.TrimSpace(asString(e["entity"])); n != "" && normalizeSurfaceForm(n) != canonicalNameKey {
			aliases = append(aliases, n)
		}
		if n := strings.TrimSpace(asString(e["entity_en"])); n != "" && normalizeSurfaceForm(n) != canonicalNameEnKey {
			aliasesEn = append(aliasesEn, n)
		}
		aliases = append(aliases, toStringSlice(e["aliases"])...)
		aliasesEn = append(aliasesEn, toStringSlice(e["aliases_en"])...)
		keywords = append(keywords, toStringSlice(e["keywords"])...)
		keywordsEn = append(keywordsEn, toStringSlice(e["keywords_en"])...)
		spans = append(spans, toStringSlice(e["line_spans"])...)
		categories = append(categories, toStringSlice(e["entity_categories"])...)

		if c := toFloat(e["confidence"]); c > maxConf {
			maxConf = c
		}
		if cs, ok := chunkSeqNoOf(e); ok && (minChunk < 0 || cs < minChunk) {
			minChunk = cs
		}
	}

	merged["aliases"] = dedupKeepOrder(aliases, canonicalNameKey)
	merged["aliases_en"] = dedupKeepOrder(aliasesEn, canonicalNameEnKey)
	merged["keywords"] = dedupKeepOrder(keywords, "")
	merged["keywords_en"] = dedupKeepOrder(keywordsEn, "")
	merged["entity_categories"] = dedupKeepOrder(categories, "")
	merged["line_spans"] = sortedUniqueSpans(spans)
	merged["confidence"] = maxConf
	if minChunk >= 0 {
		merged["chunk_seq_no"] = minChunk
	} else {
		merged["chunk_seq_no"] = 0
	}
	return merged
}

// pickCanonicalName returns the fullest (longest) value of key across members,
// breaking ties by higher confidence then earliest index. Returns "" if all
// members are blank for that key.
func pickCanonicalName(entities []map[string]any, members []int, key string) string {
	best := ""
	bestLen := -1
	bestConf := 0.0
	for _, idx := range members {
		v := strings.TrimSpace(asString(entities[idx][key]))
		if v == "" {
			continue
		}
		conf := toFloat(entities[idx]["confidence"])
		l := len([]rune(v))
		if l > bestLen || (l == bestLen && conf > bestConf) {
			best, bestLen, bestConf = v, l, conf
		}
	}
	return best
}

func fillBlank(dst map[string]any, key string, src map[string]any) {
	if strings.TrimSpace(asString(dst[key])) != "" {
		return
	}
	if v := strings.TrimSpace(asString(src[key])); v != "" {
		dst[key] = v
	}
}

func chunkSeqNoOf(e map[string]any) (int, bool) {
	switch v := e["chunk_seq_no"].(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	}
	return 0, false
}

// dedupKeepOrder removes duplicates (by normalized form) and the optional
// exclude key, preserving first-seen order of the original strings.
func dedupKeepOrder(items []string, excludeNormalized string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(items))
	for _, s := range items {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		n := normalizeSurfaceForm(s)
		if n == excludeNormalized && excludeNormalized != "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, s)
	}
	return out
}

// sortedUniqueSpans dedups span strings and orders them by numeric start line.
func sortedUniqueSpans(spans []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(spans))
	for _, s := range spans {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		si, _ := parseLineSpanRange(out[i])
		sj, _ := parseLineSpanRange(out[j])
		return si < sj
	})
	return out
}

// ---- D5: positional windowing ----

// entitySpanInterval returns an entity's [minStart, maxEnd] over its line_spans.
// ok is false when the entity has no parseable spans.
func entitySpanInterval(e map[string]any) (minStart, maxEnd int, ok bool) {
	for _, span := range toStringSlice(e["line_spans"]) {
		st, en := parseLineSpanRange(span)
		if st <= 0 {
			continue
		}
		if !ok {
			minStart, maxEnd, ok = st, en, true
			continue
		}
		if st < minStart {
			minStart = st
		}
		if en > maxEnd {
			maxEnd = en
		}
	}
	return minStart, maxEnd, ok
}

// buildRelationWindows partitions entities into overlapping positional windows
// (D5). A window covers windowSize document lines; consecutive windows step by
// (windowSize - overlap) lines, so adjacent windows share an `overlap`-line band
// and a relation straddling a boundary lands in a window holding both endpoints.
//
// An entity belongs to a window when any of its line spans intersects the
// window. Entities without parseable spans cannot be placed positionally and are
// included in every window so relations involving them remain discoverable.
// When no entity has a span, a single window containing all entities is returned.
func buildRelationWindows(entities []map[string]any, windowSize, overlap int) [][]map[string]any {
	if len(entities) == 0 {
		return nil
	}
	if windowSize <= 0 {
		windowSize = 200
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= windowSize {
		overlap = windowSize - 1
	}

	var spanless []map[string]any
	var positioned []map[string]any
	minLine, maxLine := 0, 0
	haveBounds := false
	for _, e := range entities {
		st, en, ok := entitySpanInterval(e)
		if !ok {
			spanless = append(spanless, e)
			continue
		}
		positioned = append(positioned, e)
		if !haveBounds {
			minLine, maxLine, haveBounds = st, en, true
			continue
		}
		if st < minLine {
			minLine = st
		}
		if en > maxLine {
			maxLine = en
		}
	}

	if !haveBounds {
		// Nothing positioned: one window with everything.
		return [][]map[string]any{entities}
	}

	step := windowSize - overlap
	if step <= 0 {
		step = 1
	}

	var windows [][]map[string]any
	var prevKey string
	for lo := minLine; lo <= maxLine; lo += step {
		hi := lo + windowSize - 1
		window := make([]map[string]any, 0)
		var keyParts []string
		for i, e := range positioned {
			st, en, _ := entitySpanInterval(e)
			if st <= hi && en >= lo {
				window = append(window, e)
				keyParts = append(keyParts, fmt.Sprintf("%d", i))
			}
		}
		if len(window) == 0 {
			continue
		}
		// Skip windows identical to the previous one (clustered entities).
		key := strings.Join(keyParts, ",")
		if key == prevKey {
			continue
		}
		prevKey = key
		window = append(window, spanless...)
		windows = append(windows, window)
	}

	if len(windows) == 0 {
		// All positioned entities fell outside any step (degenerate); one window.
		return [][]map[string]any{entities}
	}
	return windows
}

// buildRelationWindowInputText renders a window's entity list (id, name, type,
// aliases, and materialized entity_context) as the Phase 2 LLM input. The model
// is instructed (by the relation prompt) to connect only entities in this list.
func buildRelationWindowInputText(window []map[string]any) string {
	var b strings.Builder
	b.WriteString("Entities in this window:\n\n")
	for _, e := range window {
		id := strings.TrimSpace(asString(e["entity_id"]))
		name := strings.TrimSpace(asString(e["entity"]))
		b.WriteString("- [")
		b.WriteString(id)
		b.WriteString("] ")
		b.WriteString(name)
		if t := strings.TrimSpace(asString(e["entity_type"])); t != "" {
			b.WriteString(" (")
			b.WriteString(t)
			b.WriteString(")")
		}
		if aliases := toStringSlice(e["aliases"]); len(aliases) > 0 {
			b.WriteString(" | aliases: ")
			b.WriteString(strings.Join(aliases, ", "))
		}
		b.WriteString("\n")
		if c := strings.TrimSpace(asString(e["entity_context"])); c != "" {
			b.WriteString("  context:\n")
			for _, line := range strings.Split(c, "\n") {
				b.WriteString("    ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ---- D4: endpoint resolution + provisional entities ----

// entityResolutionIndex maps a normalized surface form to a canonical entity_id.
type entityResolutionIndex map[string]string

// buildEntityResolutionIndex indexes consolidated entities by every surface form
// they expose. First writer wins on collision (earlier entities are canonical).
func buildEntityResolutionIndex(entities []map[string]any) entityResolutionIndex {
	idx := make(entityResolutionIndex)
	for _, e := range entities {
		id := strings.TrimSpace(asString(e["entity_id"]))
		if id == "" {
			continue
		}
		for _, f := range entitySurfaceForms(e) {
			if _, ok := idx[f]; !ok {
				idx[f] = id
			}
		}
	}
	return idx
}

// resolveAndLinkRelations resolves each relation's subject/object to a canonical
// entity_id (D1). Unresolved endpoints are promoted to provisional entities
// (D4) rather than dropped; the relation then links to the new entity_id.
// Returns the linked+deduplicated relations and the (possibly extended) entity
// slice. nextEntityIndex is the next sequential index for minting entity_ids
// (e.g. len(entities)+1) and createTime stamps any provisional rows.
func resolveAndLinkRelations(
	relations []map[string]any,
	entities []map[string]any,
	idx entityResolutionIndex,
	recordID int64,
	nextEntityIndex int,
	createTime string,
) ([]map[string]any, []map[string]any) {
	// id -> entity, so a relation can inherit line grounding from its endpoints.
	idToEntity := make(map[string]map[string]any, len(entities))
	for _, e := range entities {
		if id := strings.TrimSpace(asString(e["entity_id"])); id != "" {
			idToEntity[id] = e
		}
	}

	resolve := func(surface string) string {
		f := normalizeSurfaceForm(surface)
		if f == "" {
			return ""
		}
		if id, ok := idx[f]; ok {
			return id
		}
		// Mint a provisional entity for the unresolved endpoint.
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
			"desc":              "",
			"desc_en":           "",
			"keywords":          []string{},
			"keywords_en":       []string{},
			"line_spans":        []string{},
			"entity_categories": []string{},
			"confidence":        0.0,
			"chunk_seq_no":      0,
			"entity_status":     entityStatusProvisional,
			"create_time":       createTime,
		}
		entities = append(entities, prov)
		idToEntity[id] = prov
		idx[f] = id
		return id
	}

	deduped := make([]map[string]any, 0, len(relations))
	seen := make(map[string]struct{})
	for _, r := range relations {
		subjID := resolve(asString(r["subject"]))
		objID := resolve(asString(r["object"]))
		r["subject_entity_id"] = subjID
		r["object_entity_id"] = objID

		// The Phase 2 window input carries entity context text but not line
		// numbers, so the model cannot cite `lines` reliably. Ground the relation
		// at the union of its endpoints' spans, which downstream search indexing
		// uses to resolve chunks / semantic projections.
		if derived := relationSpansFromEndpoints(idToEntity[subjID], idToEntity[objID]); len(derived) > 0 {
			r["line_spans"] = derived
		}

		key := subjID + "\x00" + normalizePredicate(asString(r["predicate"])) + "\x00" + objID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, r)
	}

	// Provisional entities have no spans of their own; ground each at the lines of
	// the relations that reference it, so it indexes cleanly (matches the relation
	// grounding above). A provisional entity referenced only by spanless relations
	// stays ungrounded (genuinely no evidence in the document).
	backfillProvisionalEntitySpans(deduped, idToEntity)

	return deduped, entities
}

// backfillProvisionalEntitySpans sets each provisional entity's line_spans to the
// union of the spans of every relation that references it.
func backfillProvisionalEntitySpans(relations []map[string]any, idToEntity map[string]map[string]any) {
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
			e["line_spans"] = sortedUniqueSpans(append(toStringSlice(e["line_spans"]), spans...))
		}
	}
}

// relationSpansFromEndpoints returns the sorted union of the subject and object
// entities' line_spans, used to ground a relation that has no spans of its own.
func relationSpansFromEndpoints(subject, object map[string]any) []string {
	var spans []string
	if subject != nil {
		spans = append(spans, toStringSlice(subject["line_spans"])...)
	}
	if object != nil {
		spans = append(spans, toStringSlice(object["line_spans"])...)
	}
	return sortedUniqueSpans(spans)
}

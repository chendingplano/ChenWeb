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

// relationWindow is one Phase 2 unit (Change 01, 2026-06-14): the entities
// positioned in a contiguous page range, together with the line-file text for
// that range. The window carries the source text once (contiguous) instead of a
// per-entity entity_context, which removes the large per-entity duplication and
// preserves the connective prose in which relations are actually stated.
type relationWindow struct {
	Entities []map[string]any
	Lines    []Line // contiguous source lines covering [PageLo, PageHi]
	PageLo   int
	PageHi   int
}

// buildRelationWindows partitions entities into overlapping positional windows
// (D5). RELATION_WINDOW_SIZE is expressed in *pages* (windowPages); each window
// covers a contiguous page range and carries the line-file text for that range
// (Change 01). Adjacent windows share an `overlap`-*line* band at the page
// boundary, so a relation straddling the boundary still lands in a window holding
// both endpoints.
//
// An entity belongs to a window when any of its line spans intersects the
// window's line range. Entities without parseable spans cannot be placed
// positionally and are included in every window so relations involving them
// remain discoverable. When no positional/line information is available, a single
// window containing all entities (and all lines) is returned.
func buildRelationWindows(entities []map[string]any, allLines []Line, windowPages, overlap int) []relationWindow {
	if len(entities) == 0 {
		return nil
	}
	if windowPages <= 0 {
		windowPages = 20
	}
	if overlap < 0 {
		overlap = 0
	}

	// Sort lines by LineNo and index each page's [first,last] line bounds.
	lines := make([]Line, len(allLines))
	copy(lines, allLines)
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].LineNo < lines[j].LineNo })

	type pageBound struct{ first, last int }
	bounds := make(map[int]pageBound)
	var pages []int
	for _, l := range lines {
		if b, ok := bounds[l.PageNo]; ok {
			if l.LineNo < b.first {
				b.first = l.LineNo
			}
			if l.LineNo > b.last {
				b.last = l.LineNo
			}
			bounds[l.PageNo] = b
		} else {
			bounds[l.PageNo] = pageBound{l.LineNo, l.LineNo}
			pages = append(pages, l.PageNo)
		}
	}
	sort.Ints(pages)

	// Degenerate: no usable page/line info -> one window with everything.
	if len(pages) == 0 {
		return []relationWindow{{Entities: entities, Lines: lines}}
	}
	minLine := lines[0].LineNo

	var spanless []map[string]any
	for _, e := range entities {
		if _, _, ok := entitySpanInterval(e); !ok {
			spanless = append(spanless, e)
		}
	}

	var windows []relationWindow
	var prevKey string
	for i := 0; i < len(pages); i += windowPages {
		pLo := pages[i]
		hiIdx := i + windowPages - 1
		if hiIdx >= len(pages) {
			hiIdx = len(pages) - 1
		}
		pHi := pages[hiIdx]

		loLine := bounds[pLo].first - overlap
		if loLine < minLine {
			loLine = minLine
		}
		hiLine := bounds[pHi].last

		// Positioned entities whose span intersects this window's line range.
		var winEntities []map[string]any
		var keyParts []string
		for idx, e := range entities {
			st, en, ok := entitySpanInterval(e)
			if !ok {
				continue
			}
			if st <= hiLine && en >= loLine {
				winEntities = append(winEntities, e)
				keyParts = append(keyParts, fmt.Sprintf("%d", idx))
			}
		}
		if len(winEntities) == 0 {
			continue
		}
		// Skip windows whose positioned-entity set repeats the previous one
		// (clustered entities spanning several empty page-steps).
		key := strings.Join(keyParts, ",")
		if key == prevKey {
			continue
		}
		prevKey = key

		// Materialize the contiguous source text for the window's line range.
		var winLines []Line
		for _, l := range lines {
			if l.LineNo >= loLine && l.LineNo <= hiLine {
				winLines = append(winLines, l)
			}
		}

		winEntities = append(winEntities, spanless...)
		windows = append(windows, relationWindow{
			Entities: winEntities,
			Lines:    winLines,
			PageLo:   pLo,
			PageHi:   pHi,
		})
	}

	if len(windows) == 0 {
		// All positioned entities fell outside any window (degenerate); one window.
		return []relationWindow{{Entities: entities, Lines: lines}}
	}
	return windows
}

// buildRelationWindowInputText renders a window as the Phase 2 LLM input: the
// entity roster (id, name, type, aliases — so relations stay entity-linked per
// D1/D4) followed by the contiguous source text for the window's page range
// (Change 01). The model is instructed (by the relation prompt) to connect only
// entities in the roster, using the source text as evidence.
func buildRelationWindowInputText(w relationWindow) string {
	var b strings.Builder
	b.WriteString("Entities in this window:\n\n")
	for _, e := range w.Entities {
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
	}

	b.WriteString("\nSource text")
	switch {
	case w.PageLo > 0 && w.PageHi > w.PageLo:
		fmt.Fprintf(&b, " (pages %d-%d)", w.PageLo, w.PageHi)
	case w.PageLo > 0:
		fmt.Fprintf(&b, " (page %d)", w.PageLo)
	}
	b.WriteString(":\n\n")
	for _, l := range w.Lines {
		if c := strings.TrimSpace(l.Content); c != "" {
			b.WriteString(c)
			b.WriteString("\n")
		}
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

// stripCategorySuffix removes a trailing " (category)" appended by the LLM, e.g.
// "GB/T 1.1-2009 (标准)" → ("GB/T 1.1-2009", true). Returns (s, false) unchanged
// when no such suffix is present.
func stripCategorySuffix(s string) (string, bool) {
	if !strings.HasSuffix(s, ")") {
		return s, false
	}
	i := strings.LastIndex(s, "(")
	if i < 1 {
		return s, false
	}
	stripped := strings.TrimSpace(s[:i])
	if stripped == "" {
		return s, false
	}
	return stripped, true
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
		// The LLM sometimes appends " (category)" to entity names in relations.
		// Strip the suffix and retry before minting a provisional entity.
		if stripped, ok := stripCategorySuffix(strings.TrimSpace(surface)); ok {
			if fs := normalizeSurfaceForm(stripped); fs != "" {
				if id, ok2 := idx[fs]; ok2 {
					return id
				}
			}
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

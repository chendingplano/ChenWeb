package docsync

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/chendingplano/deepdoc/server/api/ontology/candidates"
	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
)

var artifactIdentityFields = map[string]string{
	"ex_summaries":                    "summary_id",
	"summaries":                       "summary_id",
	"topics":                          "topic_id",
	"metrics":                         "metric_id",
	"metrics_dropped":                 "drop_id candidate_id",
	"provisions":                      "prov_id",
	"products":                        "product_rel_id",
	"ref_mentions":                    "ref_id",
	"reference_artifact_links":        "resolved_artifact_id",
	"entities":                        "entity_id canonical_entity_id",
	"relations":                       "relation_id subject_entity_id object_entity_id",
	"inventory_items":                 "inventory_item_id canonical_item_id",
	"inventory_item_duplicates":       "inventory_item_id duplicate_of",
	"semantic_projections":            "semantic_proj_id",
	"knowledges":                      "knowledge_id",
	"scene_objects":                   "object_id scene_id",
	"object_nodes":                    "object_id canonical_object_id merged_into",
	"artifact_objects":                "artifact_id object_id",
	"assertion_evidence":              "artifact_id artifact_object_id",
	"semantic_assertions":             "logical_identity_key subject_ref_id object_ref_id subject_object_id object_object_id",
	"semantic_decision_candidates":    "logical_identity_key source_artifact_id",
	"semantic_processing_outcomes":    "artifact_id",
	"unresolved_semantic_occurrences": "artifact_id",
}

var knownArtifactPrefix = regexp.MustCompile(`^(?:sum|xsum|exsum|top|tpc|mtc|rfm|drp|prv|prd|ref|ent|rel|inv|dup|sem|smp|spj|knw|kng|sbk|scn)_`)

func portableArtifactID(value string, from, to int64) string {
	src, dst := strconv.FormatInt(from, 10), strconv.FormatInt(to, 10)
	if tail, ok := strings.CutPrefix(value, src+"_"); ok && knownArtifactPrefix.MatchString(tail) {
		return dst + "_" + tail
	}
	if tail, ok := strings.CutPrefix(value, "obj_"+src+"_"); ok {
		return "obj_" + dst + "_" + tail
	}
	for _, prefix := range []string{"metric:", "provision:", "product_structure:"} {
		if tail, ok := strings.CutPrefix(value, prefix+src+":"); ok {
			parts := strings.Split(tail, ":")
			for i, part := range parts {
				parts[i] = portableArtifactID(part, from, to)
			}
			return prefix + dst + ":" + strings.Join(parts, ":")
		}
	}
	return value
}

func stringField(fields map[string]json.RawMessage, column string) string {
	var value string
	_ = json.Unmarshal(fields[column], &value)
	return value
}

func setStringField(fields map[string]json.RawMessage, column, value string) {
	fields[column], _ = json.Marshal(value)
}

func rewriteIDField(fields map[string]json.RawMessage, column string, from, to int64) {
	if value := stringField(fields, column); value != "" {
		setStringField(fields, column, portableArtifactID(value, from, to))
	}
}

func (im *bundleImporter) rewriteFields(table tableContract, fields map[string]json.RawMessage, row PortableRow) error {
	for _, col := range strings.Fields(artifactIdentityFields[table.name]) {
		if err := im.rewriteObjectOwners(fields, col); err != nil {
			return err
		}
		rewriteIDField(fields, col, im.source, im.local)
	}
	switch table.name {
	case "ex_summaries", "summaries":
		if raw, ok := fields["child_summary_ids"]; ok {
			var children []string
			if err := json.Unmarshal(raw, &children); err != nil {
				return err
			}
			for i, child := range children {
				children[i] = portableArtifactID(child, im.source, im.local)
			}
			fields["child_summary_ids"], _ = json.Marshal(children)
		}
	case "provisions":
		if stringField(fields, "input_filename") == fmt.Sprintf("record_%d", im.source) {
			setStringField(fields, "input_filename", fmt.Sprintf("record_%d", im.local))
		}
	case "doc_facets":
		var ksStoreID int64
		if err := im.tx.QueryRowContext(im.ctx, "SELECT COALESCE(ks_store_id,0) FROM kb.inputs WHERE id=$1", im.local).Scan(&ksStoreID); err != nil {
			return err
		}
		binding := "absent"
		if ksStoreID > 0 {
			binding = "bound"
		}
		fields["ks_store_id"] = json.RawMessage(strconv.FormatInt(ksStoreID, 10))
		setStringField(fields, "knowledge_store_binding", binding)
	case "metrics", "reference_artifact_links":
		targetColumn, artifactColumn := "ref_input_record_id", "ref_metric_id"
		if table.name == "reference_artifact_links" {
			targetColumn, artifactColumn = "target_input_id", "target_artifact_id"
		}
		from, _ := readReferenceID(row.Fields[targetColumn])
		to, _ := readReferenceID(fields[targetColumn])
		if from != 0 && to != 0 {
			rewriteIDField(fields, artifactColumn, from, to)
		}
	case "ontology_candidates":
		if raw := fields["proposed_payload"]; len(raw) != 0 {
			source := fmt.Sprintf("input_record:%d", im.local)
			setStringField(fields, "source_ref", source)
			fingerprint, err := candidates.Fingerprint(raw, stringField(fields, "source_type"), source, stringField(fields, "proposed_module_id"))
			if err != nil {
				return err
			}
			setStringField(fields, "fingerprint", fingerprint)
		}
	case "semantic_decision_candidates":
		if raw := fields["proposed_payload"]; len(raw) != 0 {
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(raw, &payload); err != nil {
				return err
			}
			for _, col := range []string{"subject_object_id", "object_object_id", "subject_ref_id", "object_ref_id", "metric_id", "provision_id"} {
				if err := im.rewriteObjectOwners(payload, col); err != nil {
					return err
				}
				rewriteIDField(payload, col, im.source, im.local)
			}
			fields["proposed_payload"], _ = json.Marshal(payload)
		}
	case "unresolved_semantic_occurrences":
		if len(row.Fields) != 0 {
			family := stringField(fields, "artifact_type")
			scope := (semantic.FallbackAdapter{ArtifactTypeValue: family}).OccurrenceScope()
			setStringField(fields, "occurrence_key", semantic.OccurrenceKey(scope, im.local, family, stringField(fields, "artifact_id")))
			switch stringField(fields, "materialization_state") {
			case "claimed", "materializing":
				setStringField(fields, "materialization_state", "pending")
			}
		}
	case "semantic_processing_outcomes":
		if len(row.Fields) != 0 {
			setStringField(fields, "outcome_key", semantic.OutcomeKey(im.local,
				stringField(fields, "artifact_type"), stringField(fields, "artifact_id"), stringField(fields, "stage_term_id")))
		}
	}
	return nil
}

// The referring rows provide narrow natural keys for these registries.
var artifactFamily = map[string]string{
	"extract_metrics": "metric", "extract_metrics_simple": "metric",
	"extract_entity": "entity", "extract_relation": "relation",
	"extract_provisions": "provision", "extract_inventory_items": "inventory_item",
	"extract_semantic_projections": "semantic_projection", "extract_structured_knowledge": "knowledge",
	"generate_topics": "topic", "generate_summaries": "summary", "generate_scene_blocks": "scene_block",
}

func entityRegistryKey(name string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), "_")
}

func (e *bundleExporter) collectRegistries(processor string) error {
	if family := artifactFamily[processor]; family != "" {
		rows, err := queryContractRows(e.ctx, e.tx, portableTables["artifact_categories"],
			`EXISTS (SELECT 1 FROM kb.artifact_connections e
			WHERE e.source_record_id=$1 AND e.source_type=$2 AND e.relation_method='category_name'
			  AND e.target_type=t.category_type AND e.target_id=t.category_key)`, e.input, family)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if err := e.addRegistryRow("artifact_categories", row); err != nil {
				return err
			}
		}
	}
	for _, row := range e.bundle.Rows["artifact_objects"] {
		if err := e.collectObjectNodes(stringField(row.Fields, "object_id")); err != nil {
			return err
		}
	}
	for _, row := range e.bundle.Rows["semantic_assertions"] {
		if claim, ok := strings.CutPrefix(stringField(row.Fields, "logical_identity_key"), "claim:"); ok {
			rows, err := queryContractRows(e.ctx, e.tx, portableTables["semantic_claim_identities"], "t.claim_id=$1", claim)
			if err != nil {
				return err
			}
			if len(rows) == 1 && !slices.ContainsFunc(e.bundle.Rows["semantic_claim_identities"], func(row PortableRow) bool { return stringField(row.Fields, "claim_id") == claim }) {
				e.bundle.Rows["semantic_claim_identities"] = append(e.bundle.Rows["semantic_claim_identities"], rows[0])
			}
		}
		if err := e.collectObjectNodes(stringField(row.Fields, "subject_object_id"), stringField(row.Fields, "object_object_id")); err != nil {
			return err
		}
	}
	switch processor {
	case "extract_product_structure":
		for _, row := range e.bundle.Rows["semantic_decision_candidates"] {
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(row.Fields["proposed_payload"], &payload); err != nil {
				return err
			}
			if err := e.collectObjectNodes(stringField(payload, "subject_object_id"), stringField(payload, "object_object_id")); err != nil {
				return err
			}
		}
	case "extract_references":
		for _, row := range e.bundle.Rows["ref_resources"] {
			identifiers, err := queryContractRows(e.ctx, e.tx, portableTables["ref_resource_identifiers"], "t.resource_id=$1", row.SourceID)
			if err != nil {
				return err
			}
			for _, identifier := range identifiers {
				if err := e.addRegistryRow("ref_resource_identifiers", identifier); err != nil {
					return err
				}
			}
		}
	case "extract_entity":
		for _, row := range e.bundle.Rows["entities"] {
			name := stringField(row.Fields, "entity_en")
			if strings.TrimSpace(name) == "" {
				name = stringField(row.Fields, "entity")
			}
			if err := e.collectRegistry("entity_names", "t.name_key=$1", entityRegistryKey(name)); err != nil {
				return err
			}
		}
	case "extract_relation":
		for _, row := range e.bundle.Rows["relations"] {
			predicate := stringField(row.Fields, "predicate_en")
			if strings.TrimSpace(predicate) == "" {
				predicate = stringField(row.Fields, "predicate")
			}
			if err := e.collectRegistry("relation_predicates", "t.predicate_key=$1", strings.Join(strings.Fields(strings.ToLower(predicate)), "_")); err != nil {
				return err
			}
		}
	case "extract_inventory_items":
		for _, row := range e.bundle.Rows["inventory_items"] {
			var keys []string
			if err := json.Unmarshal(row.Fields["item_categories"], &keys); err != nil {
				return err
			}
			for _, key := range keys {
				if err := e.collectRegistry("artifact_categories", "t.category_type='inventory' AND t.category_key=$1", key); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (e *bundleExporter) collectObjectNodes(ids ...string) error {
	for _, id := range ids {
		if id == "" {
			continue
		}
		owner := objectOwnerID(id)
		if _, read := e.bundle.ObjectOwners[owner]; !read && owner > 0 && owner != e.input {
			var hash string
			err := e.tx.QueryRowContext(e.ctx, "SELECT COALESCE(lower(btrim(md5)),'') FROM kb.inputs WHERE id=$1", owner).Scan(&hash)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			e.bundle.ObjectOwners[owner] = hash
		}
		if err := e.collectRegistry("object_nodes", "t.object_id=$1", id); err != nil {
			return err
		}
	}
	return nil
}

func (e *bundleExporter) collectRegistry(name, where string, value any) error {
	rows, err := queryContractRows(e.ctx, e.tx, portableTables[name], where, value)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := e.addRegistryRow(name, row); err != nil {
			return err
		}
	}
	return nil
}

func (e *bundleExporter) addRegistryRow(name string, row PortableRow) error {
	if _, exists := e.cache[name][row.SourceID]; exists {
		return nil
	}
	e.remember(name, row)
	if err := e.enrich(name, &row); err != nil {
		return err
	}
	e.bundle.Rows[name] = append(e.bundle.Rows[name], row)
	if name == "object_nodes" {
		return e.collectObjectNodes(stringField(row.Fields, "canonical_object_id"), stringField(row.Fields, "merged_into"))
	}
	return nil
}

func objectOwnerID(value string) int64 {
	tail, ok := strings.CutPrefix(value, "obj_")
	if !ok {
		return 0
	}
	before, _, ok := strings.Cut(tail, "_")
	if !ok {
		return 0
	}
	id, _ := strconv.ParseInt(before, 10, 64)
	return id
}

func (im *bundleImporter) rewriteObjectOwners(fields map[string]json.RawMessage, column string) error {
	value := stringField(fields, column)
	parts := strings.Split(value, ":")
	for i, part := range parts {
		owner := objectOwnerID(part)
		if owner <= 0 || owner == im.source {
			continue
		}
		hash := im.objectOwners[owner]
		if hash == "" {
			continue
		}
		local := im.resolvedOwners[owner]
		if local == 0 {
			raw, _ := json.Marshal(hash)
			var err error
			local, err = im.lookup("inputs", owner, raw)
			if err != nil {
				return err
			}
			im.resolvedOwners[owner] = local
		}
		parts[i] = portableArtifactID(part, owner, local)
	}
	if value != "" {
		setStringField(fields, column, strings.Join(parts, ":"))
	}
	return nil
}

func validateRowOwner(processor, table string, fields map[string]json.RawMessage) error {
	valid := true
	switch table {
	case "doc_facet_values":
		method := map[string]string{"facet_tier1": "deterministic", "facet_tier2": "metadata", "classify_document": "classifier"}[processor]
		valid = stringField(fields, "method") == method
	case "metrics":
		ref, _ := readReferenceID(fields["ref_mention_id"])
		valid = (ref != 0) == (processor == "extract_references")
	case "ontology_candidates":
		valid = stringField(fields, "proposed_by") == processor && stringField(fields, "source_type") == "document"
	case "artifact_objects":
		valid = stringField(fields, "artifact_type") == artifactFamily[processor]
	case "semantic_decision_candidates":
		valid = (stringField(fields, "source_artifact_type") == "product_structure") == (processor == "extract_product_structure")
	}
	if !valid {
		return fmt.Errorf("%s row is outside the %s ownership scope", table, processor)
	}
	return nil
}

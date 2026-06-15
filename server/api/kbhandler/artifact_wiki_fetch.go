package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func fetchSemanticProjectionByID(db *sql.DB, artifactID string) (semanticProjectionRecord, error) {
	const q = `
SELECT
	id, semantic_proj_id, input_record_id, event_id, language,
	descriptive_name, descriptive_name_en, keywords, keywords_en,
	semantic_projection, semantic_projection_en, category_paths, category_paths_en,
	line_spans,
	model_name, prompt_name, create_time
FROM kb.semantic_projections
WHERE semantic_proj_id = $1
LIMIT 1`
	var (
		r                    semanticProjectionRecord
		eventID              sql.NullString
		language             sql.NullString
		descriptiveName      sql.NullString
		descriptiveNameEn    sql.NullString
		keywords             []byte
		keywordsEn           []byte
		semanticProjection   sql.NullString
		semanticProjectionEn sql.NullString
		categoryPaths        []byte
		categoryPathsEn      []byte
		lineSpans            []byte
		modelName            sql.NullString
		promptName           sql.NullString
		createTime           time.Time
	)
	if err := db.QueryRow(q, artifactID).Scan(
		&r.ID, &r.SemanticProjID, &r.InputRecordID, &eventID, &language,
		&descriptiveName, &descriptiveNameEn, &keywords, &keywordsEn,
		&semanticProjection, &semanticProjectionEn, &categoryPaths, &categoryPathsEn,
		&lineSpans, &modelName, &promptName, &createTime,
	); err != nil {
		return semanticProjectionRecord{}, err
	}
	r.EventID = eventID.String
	r.Language = language.String
	r.DescriptiveName = descriptiveName.String
	r.DescriptiveNameEn = descriptiveNameEn.String
	r.SemanticProjection = semanticProjection.String
	r.SemanticProjectionEn = semanticProjectionEn.String
	r.Keywords = jsonArrayOrEmpty(keywords)
	r.KeywordsEn = jsonArrayOrEmpty(keywordsEn)
	r.CategoryPaths = jsonArrayOrEmpty(categoryPaths)
	r.CategoryPathsEn = jsonArrayOrEmpty(categoryPathsEn)
	r.LineSpans = jsonArrayOrEmpty(lineSpans)
	r.ModelName = modelName.String
	r.PromptName = promptName.String
	r.CreateTime = createTime.Format(time.RFC3339)
	return r, nil
}

func fetchSceneBlockByObjectID(db *sql.DB, artifactID string) (sceneBlockRecord, error) {
	const q = `
SELECT
	id, object_id, event_id, scene_id, scene_type, title, summary,
	actors, resources, preconditions, triggers, states, actions, constraints,
	decisions, outcomes, failure_modes, root_causes, resolutions, relationships,
	discriminators, keywords, confidence, model_name, prompt_name, ext_info,
	line_spans,
	create_time, modify_time
FROM kb.scene_objects
WHERE object_id = $1
LIMIT 1`
	var (
		r          sceneBlockRecord
		actors     []byte
		resources  []byte
		precond    []byte
		triggers   []byte
		states     []byte
		actions    []byte
		constr     []byte
		decisions  []byte
		outcomes   []byte
		failure    []byte
		rootCauses []byte
		resolution []byte
		relations  []byte
		discrim    []byte
		keywords   []byte
		extInfo    []byte
		lineSpans  []byte
		createTime time.Time
		modifyTime time.Time
	)
	if err := db.QueryRow(q, artifactID).Scan(
		&r.ID, &r.ObjectID, &r.EventID, &r.SceneID, &r.SceneType, &r.Title, &r.Summary,
		&actors, &resources, &precond, &triggers, &states, &actions, &constr,
		&decisions, &outcomes, &failure, &rootCauses, &resolution, &relations,
		&discrim, &keywords, &r.Confidence, &r.ModelName, &r.PromptName, &extInfo,
		&lineSpans, &createTime, &modifyTime,
	); err != nil {
		return sceneBlockRecord{}, err
	}
	r.Actors = jsonArrayOrEmpty(actors)
	r.Resources = jsonArrayOrEmpty(resources)
	r.Preconditions = jsonArrayOrEmpty(precond)
	r.Triggers = jsonArrayOrEmpty(triggers)
	r.States = jsonArrayOrEmpty(states)
	r.Actions = jsonArrayOrEmpty(actions)
	r.Constraints = jsonArrayOrEmpty(constr)
	r.Decisions = jsonArrayOrEmpty(decisions)
	r.Outcomes = jsonArrayOrEmpty(outcomes)
	r.FailureModes = jsonArrayOrEmpty(failure)
	r.RootCauses = jsonArrayOrEmpty(rootCauses)
	r.Resolutions = jsonArrayOrEmpty(resolution)
	r.Relationships = jsonArrayOrEmpty(relations)
	r.Discriminators = jsonArrayOrEmpty(discrim)
	r.Keywords = jsonArrayOrEmpty(keywords)
	applySceneBlockExtInfo(&r, extInfo)
	r.LineSpans = jsonArrayOrEmpty(lineSpans)
	r.CreateTime = createTime.Format(time.RFC3339)
	r.ModifyTime = modifyTime.Format(time.RFC3339)
	return r, nil
}

func fetchInputRecordIDBySceneObjectID(db *sql.DB, artifactID string) (int64, error) {
	var inputID int64
	if err := db.QueryRow(`SELECT input_record_id FROM kb.scene_objects WHERE object_id = $1 LIMIT 1`, artifactID).Scan(&inputID); err != nil {
		return 0, err
	}
	return inputID, nil
}

func fetchProductByRelID(db *sql.DB, artifactID string) (productRecord, int64, error) {
	const q = `
SELECT
	id, input_record_id, product_rel_id, product_name, product_name_en,
	canonical_name, canonical_name_en, product_type, relation_type,
	relation_summary, relation_summary_en, evidence_quote, evidence_lines,
	obligation_level, requirement_text, requirement_text_en,
	conditions, exceptions, parameters, related_products,
	responsible_actor, confidence, confidence_reason,
	model_name, prompt_name, create_time, modify_time
FROM kb.products
WHERE product_rel_id = $1
LIMIT 1`
	var (
		r          productRecord
		inputID    int64
		evidence   []byte
		conditions []byte
		exceptions []byte
		parameters []byte
		related    []byte
		createTime time.Time
		modifyTime time.Time
	)
	if err := db.QueryRow(q, artifactID).Scan(
		&r.ID, &inputID, &r.ProductRelID, &r.ProductName, &r.ProductNameEn,
		&r.CanonicalName, &r.CanonicalNameEn, &r.ProductType, &r.RelationType,
		&r.RelationSummary, &r.RelationSummaryEn, &r.EvidenceQuote, &evidence,
		&r.ObligationLevel, &r.RequirementText, &r.RequirementTextEn,
		&conditions, &exceptions, &parameters, &related,
		&r.ResponsibleActor, &r.Confidence, &r.ConfidenceReason,
		&r.ModelName, &r.PromptName, &createTime, &modifyTime,
	); err != nil {
		return productRecord{}, 0, err
	}
	r.EvidenceLines = jsonArrayOrEmpty(evidence)
	r.Conditions = jsonArrayOrEmpty(conditions)
	r.Exceptions = jsonArrayOrEmpty(exceptions)
	r.Parameters = jsonArrayOrEmpty(parameters)
	r.RelatedProducts = jsonArrayOrEmpty(related)
	r.CreateTime = createTime.Format(time.RFC3339)
	r.ModifyTime = modifyTime.Format(time.RFC3339)
	return r, inputID, nil
}

func fetchInventoryItemByArtifactID(db *sql.DB, artifactID string) (inventoryItemRecord, int64, error) {
	const q = `
SELECT
	id, input_record_id, inventory_item_id, item_name, canonical_name, item_categories,
	manufacturer, brand, model_number, part_number,
	normalized_specs, raw_specs, standards, aliases,
	evidence_quote, source_line_spans, validation_flags, missing_required_attrs,
	dedupe_key, schema_version, dictionary_version, confidence, confidence_reason,
	model_name, prompt_name, create_time, modify_time
FROM kb.inventory_items
WHERE inventory_item_id = $1
LIMIT 1`
	var (
		r                    inventoryItemRecord
		inputID              int64
		itemCategories       []byte
		normalizedSpecs      []byte
		rawSpecs             []byte
		standards            []byte
		aliases              []byte
		sourceLineSpans      []byte
		validationFlags      []byte
		missingRequiredAttrs []byte
		createTime           time.Time
		modifyTime           time.Time
	)
	if err := db.QueryRow(q, artifactID).Scan(
		&r.ID, &inputID, &r.InventoryItemID, &r.ItemName, &r.CanonicalName, &itemCategories,
		&r.Manufacturer, &r.Brand, &r.ModelNumber, &r.PartNumber,
		&normalizedSpecs, &rawSpecs, &standards, &aliases,
		&r.EvidenceQuote, &sourceLineSpans, &validationFlags, &missingRequiredAttrs,
		&r.DedupeKey, &r.SchemaVersion, &r.DictionaryVersion, &r.Confidence, &r.ConfidenceReason,
		&r.ModelName, &r.PromptName, &createTime, &modifyTime,
	); err != nil {
		return inventoryItemRecord{}, 0, err
	}
	r.ItemCategories = jsonArrayOrEmpty(itemCategories)
	r.NormalizedSpecs = jsonArrayOrEmpty(normalizedSpecs)
	r.RawSpecs = jsonArrayOrEmpty(rawSpecs)
	r.Standards = jsonArrayOrEmpty(standards)
	r.Aliases = jsonArrayOrEmpty(aliases)
	r.SourceLineSpans = jsonArrayOrEmpty(sourceLineSpans)
	r.ValidationFlags = jsonArrayOrEmpty(validationFlags)
	r.MissingRequiredAttrs = jsonArrayOrEmpty(missingRequiredAttrs)
	r.CreateTime = createTime.Format(time.RFC3339)
	r.ModifyTime = modifyTime.Format(time.RFC3339)
	return r, inputID, nil
}

func fetchProvisionByProvID(db *sql.DB, artifactID string) (provisionRecordJSON, error) {
	const q = `
SELECT
    p.id, p.input_record_id, p.prov_id, COALESCE(p.input_filename, '') AS input_filename,
    p.prov_name, p.prov_name_en, p.provision_type, p.source_text, p.source_line_spans,
    p.provision, p.provision_en, p.provision_subject, p.provision_subject_en,
    p.prov_desc, p.prov_desc_en, p.prov_context, p.prov_context_en,
    p.provision_keywords, p.provision_keywords_en, p.category_paths, p.category_paths_en,
    p.location_type, p.confidence, p.is_explicit, p.need_verify, p.model_name, p.prompt_name,
    COALESCE(to_char(p.create_time, 'YYYY-MM-DD\"T\"HH24:MI:SSOF'), '') AS created_at
FROM kb.provisions p
WHERE p.prov_id = $1
LIMIT 1`
	var (
		r               provisionRecordJSON
		spansBytes      []byte
		keywordsBytes   []byte
		keywordsEnBytes []byte
		categoryBytes   []byte
		categoryEnBytes []byte
		confidence      sql.NullFloat64
		isExplicit      sql.NullBool
		needVerify      sql.NullBool
	)
	if err := db.QueryRow(q, artifactID).Scan(
		&r.ID, &r.InputRecordID, &r.ProvID, &r.InputFilename,
		&r.ProvName, &r.ProvNameEn, &r.ProvisionType, &r.SourceText, &spansBytes,
		&r.Provision, &r.ProvisionEn, &r.ProvisionSubject, &r.ProvisionSubjectEn,
		&r.ProvDesc, &r.ProvDescEn, &r.ProvContext, &r.ProvContextEn,
		&keywordsBytes, &keywordsEnBytes, &categoryBytes, &categoryEnBytes,
		&r.LocationType, &confidence, &isExplicit, &needVerify, &r.ModelName, &r.PromptName,
		&r.CreatedAt,
	); err != nil {
		return provisionRecordJSON{}, err
	}
	r.SourceLineSpans = jsonArrayOrEmpty(spansBytes)
	r.ProvisionKeywords = jsonArrayOrEmpty(keywordsBytes)
	r.ProvisionKeywordsEn = jsonArrayOrEmpty(keywordsEnBytes)
	r.CategoryPaths = jsonArrayOrEmpty(categoryBytes)
	r.CategoryPathsEn = jsonArrayOrEmpty(categoryEnBytes)
	if confidence.Valid {
		v := confidence.Float64
		r.Confidence = &v
	}
	if isExplicit.Valid {
		v := isExplicit.Bool
		r.IsExplicit = &v
	}
	if needVerify.Valid {
		v := needVerify.Bool
		r.NeedVerify = &v
	}
	return r, nil
}

func fetchSceneBlockByArtifactID(db *sql.DB, artifactID string) (sceneBlockRecord, int64, error) {
	recordID, seqNo, err := parseArtifactSearchIDSeq(artifactID, "scene_block")
	if err != nil {
		return sceneBlockRecord{}, 0, err
	}
	const q = `
SELECT
	id, object_id, event_id, scene_id, scene_type, title, summary,
	actors, resources, preconditions, triggers, states, actions, constraints,
	decisions, outcomes, failure_modes, root_causes, resolutions, relationships,
	discriminators, keywords, confidence, model_name, prompt_name, ext_info,
	line_spans,
	create_time, modify_time
FROM kb.scene_objects
WHERE input_record_id = $1
  AND (object_id = $2 OR regexp_replace(object_id, '^.*_', '') = $3)
ORDER BY CASE WHEN object_id = $2 THEN 0 ELSE 1 END, id
LIMIT 1`
	var (
		r          sceneBlockRecord
		actors     []byte
		resources  []byte
		precond    []byte
		triggers   []byte
		states     []byte
		actions    []byte
		constr     []byte
		decisions  []byte
		outcomes   []byte
		failure    []byte
		rootCauses []byte
		resolution []byte
		relations  []byte
		discrim    []byte
		keywords   []byte
		extInfo    []byte
		lineSpans  []byte
		createTime time.Time
		modifyTime time.Time
	)
	if err := db.QueryRow(q, recordID, artifactID, fmt.Sprintf("%d", seqNo)).Scan(
		&r.ID, &r.ObjectID, &r.EventID, &r.SceneID, &r.SceneType, &r.Title, &r.Summary,
		&actors, &resources, &precond, &triggers, &states, &actions, &constr,
		&decisions, &outcomes, &failure, &rootCauses, &resolution, &relations,
		&discrim, &keywords, &r.Confidence, &r.ModelName, &r.PromptName, &extInfo,
		&lineSpans, &createTime, &modifyTime,
	); err != nil {
		return sceneBlockRecord{}, 0, err
	}
	r.Actors = jsonArrayOrEmpty(actors)
	r.Resources = jsonArrayOrEmpty(resources)
	r.Preconditions = jsonArrayOrEmpty(precond)
	r.Triggers = jsonArrayOrEmpty(triggers)
	r.States = jsonArrayOrEmpty(states)
	r.Actions = jsonArrayOrEmpty(actions)
	r.Constraints = jsonArrayOrEmpty(constr)
	r.Decisions = jsonArrayOrEmpty(decisions)
	r.Outcomes = jsonArrayOrEmpty(outcomes)
	r.FailureModes = jsonArrayOrEmpty(failure)
	r.RootCauses = jsonArrayOrEmpty(rootCauses)
	r.Resolutions = jsonArrayOrEmpty(resolution)
	r.Relationships = jsonArrayOrEmpty(relations)
	r.Discriminators = jsonArrayOrEmpty(discrim)
	r.Keywords = jsonArrayOrEmpty(keywords)
	applySceneBlockExtInfo(&r, extInfo)
	r.LineSpans = jsonArrayOrEmpty(lineSpans)
	r.CreateTime = createTime.Format(time.RFC3339)
	r.ModifyTime = modifyTime.Format(time.RFC3339)
	return r, recordID, nil
}

func fetchProductByArtifactID(db *sql.DB, artifactID string) (productRecord, int64, error) {
	recordID, seqNo, err := parseArtifactSearchIDSeq(artifactID, "product")
	if err != nil {
		return productRecord{}, 0, err
	}
	const q = `
SELECT
	id, input_record_id, product_rel_id, product_name, product_name_en,
	canonical_name, canonical_name_en, product_type, relation_type,
	relation_summary, relation_summary_en, evidence_quote, evidence_lines,
	obligation_level, requirement_text, requirement_text_en,
	conditions, exceptions, parameters, related_products,
	responsible_actor, confidence, confidence_reason,
	model_name, prompt_name, create_time, modify_time
FROM kb.products
WHERE input_record_id = $1
  AND (product_rel_id = $2 OR regexp_replace(product_rel_id, '^.*_', '') = $3)
ORDER BY CASE WHEN product_rel_id = $2 THEN 0 ELSE 1 END, id
LIMIT 1`
	var (
		r          productRecord
		inputID    int64
		evidence   []byte
		conditions []byte
		exceptions []byte
		parameters []byte
		related    []byte
		createTime time.Time
		modifyTime time.Time
	)
	if err := db.QueryRow(q, recordID, artifactID, fmt.Sprintf("%d", seqNo)).Scan(
		&r.ID, &inputID, &r.ProductRelID, &r.ProductName, &r.ProductNameEn,
		&r.CanonicalName, &r.CanonicalNameEn, &r.ProductType, &r.RelationType,
		&r.RelationSummary, &r.RelationSummaryEn, &r.EvidenceQuote, &evidence,
		&r.ObligationLevel, &r.RequirementText, &r.RequirementTextEn,
		&conditions, &exceptions, &parameters, &related,
		&r.ResponsibleActor, &r.Confidence, &r.ConfidenceReason,
		&r.ModelName, &r.PromptName, &createTime, &modifyTime,
	); err != nil {
		return productRecord{}, 0, err
	}
	r.EvidenceLines = jsonArrayOrEmpty(evidence)
	r.Conditions = jsonArrayOrEmpty(conditions)
	r.Exceptions = jsonArrayOrEmpty(exceptions)
	r.Parameters = jsonArrayOrEmpty(parameters)
	r.RelatedProducts = jsonArrayOrEmpty(related)
	r.CreateTime = createTime.Format(time.RFC3339)
	r.ModifyTime = modifyTime.Format(time.RFC3339)
	return r, inputID, nil
}

func fetchInventoryItemBySearchID(db *sql.DB, artifactID string) (inventoryItemRecord, int64, error) {
	recordID, seqNo, err := parseArtifactSearchIDSeq(artifactID, "inventory_item")
	if err != nil {
		return inventoryItemRecord{}, 0, err
	}
	const q = `
SELECT
	id, input_record_id, inventory_item_id, item_name, canonical_name, item_categories,
	manufacturer, brand, model_number, part_number,
	normalized_specs, raw_specs, standards, aliases,
	evidence_quote, source_line_spans, validation_flags, missing_required_attrs,
	dedupe_key, schema_version, dictionary_version, confidence, confidence_reason,
	model_name, prompt_name, create_time, modify_time
FROM kb.inventory_items
WHERE input_record_id = $1
  AND (inventory_item_id = $2 OR regexp_replace(inventory_item_id, '^.*_', '') = $3)
ORDER BY CASE WHEN inventory_item_id = $2 THEN 0 ELSE 1 END, id
LIMIT 1`
	var (
		r                    inventoryItemRecord
		inputID              int64
		itemCategories       []byte
		normalizedSpecs      []byte
		rawSpecs             []byte
		standards            []byte
		aliases              []byte
		sourceLineSpans      []byte
		validationFlags      []byte
		missingRequiredAttrs []byte
		createTime           time.Time
		modifyTime           time.Time
	)
	if err := db.QueryRow(q, recordID, artifactID, fmt.Sprintf("%d", seqNo)).Scan(
		&r.ID, &inputID, &r.InventoryItemID, &r.ItemName, &r.CanonicalName, &itemCategories,
		&r.Manufacturer, &r.Brand, &r.ModelNumber, &r.PartNumber,
		&normalizedSpecs, &rawSpecs, &standards, &aliases,
		&r.EvidenceQuote, &sourceLineSpans, &validationFlags, &missingRequiredAttrs,
		&r.DedupeKey, &r.SchemaVersion, &r.DictionaryVersion, &r.Confidence, &r.ConfidenceReason,
		&r.ModelName, &r.PromptName, &createTime, &modifyTime,
	); err != nil {
		return inventoryItemRecord{}, 0, err
	}
	r.ItemCategories = jsonArrayOrEmpty(itemCategories)
	r.NormalizedSpecs = jsonArrayOrEmpty(normalizedSpecs)
	r.RawSpecs = jsonArrayOrEmpty(rawSpecs)
	r.Standards = jsonArrayOrEmpty(standards)
	r.Aliases = jsonArrayOrEmpty(aliases)
	r.SourceLineSpans = jsonArrayOrEmpty(sourceLineSpans)
	r.ValidationFlags = jsonArrayOrEmpty(validationFlags)
	r.MissingRequiredAttrs = jsonArrayOrEmpty(missingRequiredAttrs)
	r.CreateTime = createTime.Format(time.RFC3339)
	r.ModifyTime = modifyTime.Format(time.RFC3339)
	return r, inputID, nil
}

func fetchProvisionByArtifactID(db *sql.DB, artifactID string) (provisionRecordJSON, error) {
	recordID, tail, err := parseArtifactSearchID(artifactID, "provision")
	if err != nil {
		return provisionRecordJSON{}, err
	}
	const q = `
SELECT
    p.id, p.input_record_id, p.prov_id, COALESCE(p.input_filename, '') AS input_filename,
    p.prov_name, p.prov_name_en, p.provision_type, p.source_text, p.source_line_spans,
    p.provision, p.provision_en, p.provision_subject, p.provision_subject_en,
    p.prov_desc, p.prov_desc_en, p.prov_context, p.prov_context_en,
    p.provision_keywords, p.provision_keywords_en, p.category_paths, p.category_paths_en,
    p.location_type, p.confidence, p.is_explicit, p.need_verify, p.model_name, p.prompt_name,
    COALESCE(to_char(p.create_time, 'YYYY-MM-DD\"T\"HH24:MI:SSOF'), '') AS created_at
FROM kb.provisions p
WHERE p.input_record_id = $1
  AND (p.prov_id = $2 OR p.prov_id = $3)
ORDER BY CASE WHEN p.prov_id = $2 THEN 0 ELSE 1 END, p.id
LIMIT 1`
	var (
		r               provisionRecordJSON
		spansBytes      []byte
		keywordsBytes   []byte
		keywordsEnBytes []byte
		categoryBytes   []byte
		categoryEnBytes []byte
		confidence      sql.NullFloat64
		isExplicit      sql.NullBool
		needVerify      sql.NullBool
	)
	if err := db.QueryRow(q, recordID, artifactID, tail).Scan(
		&r.ID, &r.InputRecordID, &r.ProvID, &r.InputFilename,
		&r.ProvName, &r.ProvNameEn, &r.ProvisionType, &r.SourceText, &spansBytes,
		&r.Provision, &r.ProvisionEn, &r.ProvisionSubject, &r.ProvisionSubjectEn,
		&r.ProvDesc, &r.ProvDescEn, &r.ProvContext, &r.ProvContextEn,
		&keywordsBytes, &keywordsEnBytes, &categoryBytes, &categoryEnBytes,
		&r.LocationType, &confidence, &isExplicit, &needVerify, &r.ModelName, &r.PromptName,
		&r.CreatedAt,
	); err != nil {
		return provisionRecordJSON{}, err
	}
	r.SourceLineSpans = jsonArrayOrEmpty(spansBytes)
	r.ProvisionKeywords = jsonArrayOrEmpty(keywordsBytes)
	r.ProvisionKeywordsEn = jsonArrayOrEmpty(keywordsEnBytes)
	r.CategoryPaths = jsonArrayOrEmpty(categoryBytes)
	r.CategoryPathsEn = jsonArrayOrEmpty(categoryEnBytes)
	if confidence.Valid {
		v := confidence.Float64
		r.Confidence = &v
	}
	if isExplicit.Valid {
		v := isExplicit.Bool
		r.IsExplicit = &v
	}
	if needVerify.Valid {
		v := needVerify.Bool
		r.NeedVerify = &v
	}
	return r, nil
}

type knowledgeRecord struct {
	ID               int64           `json:"id"`
	InputRecordID    int64           `json:"input_record_id"`
	KnowledgeID      string          `json:"knowledge_id"`
	Language         string          `json:"language"`
	KnowledgeType    string          `json:"knowledge_type"`
	KnowledgeValue   string          `json:"knowledge_value"`
	KnowledgeValueEn string          `json:"knowledge_value_en,omitempty"`
	DescText         string          `json:"desc_text"`
	DescTextEn       string          `json:"desc_text_en,omitempty"`
	Keywords         json.RawMessage `json:"keywords"`
	KeywordsEn       json.RawMessage `json:"keywords_en"`
	Lines            json.RawMessage `json:"lines"`
	CategoryPaths    json.RawMessage `json:"category_paths"`
	CategoryPathsEn  json.RawMessage `json:"category_paths_en"`
	ModelName        string          `json:"model_name"`
	PromptName       string          `json:"prompt_name"`
	SearchDocument   string          `json:"search_document,omitempty"`
	CreateTime       string          `json:"create_time"`
}

type entityRecord struct {
	ID             int64           `json:"id"`
	InputRecordID  int64           `json:"input_record_id"`
	EntityID       string          `json:"entity_id"`
	Language       string          `json:"language"`
	Entity         string          `json:"entity"`
	EntityEn       string          `json:"entity_en,omitempty"`
	EntityType     string          `json:"entity_type"`
	EntityTypeEn   string          `json:"entity_type_en,omitempty"`
	Aliases        json.RawMessage `json:"aliases"`
	AliasesEn      json.RawMessage `json:"aliases_en"`
	DescText       string          `json:"desc_text"`
	DescTextEn     string          `json:"desc_text_en,omitempty"`
	Keywords       json.RawMessage `json:"keywords"`
	KeywordsEn     json.RawMessage `json:"keywords_en"`
	LineSpans      json.RawMessage `json:"line_spans"`
	Confidence     float64         `json:"confidence"`
	ModelName      string          `json:"model_name"`
	PromptName     string          `json:"prompt_name"`
	EntityContext  string          `json:"entity_context,omitempty"`
	DocName        string          `json:"doc_name,omitempty"`
	Categories     json.RawMessage `json:"categories"`
	EntityStatus   string          `json:"entity_status"`
	SearchDocument string          `json:"search_document,omitempty"`
	Connected      json.RawMessage `json:"connected_artifacts"`
	ExtInfo        json.RawMessage `json:"ext_info"`
	CreateTime     string          `json:"create_time"`
}

type relationRecord struct {
	ID              int64           `json:"id"`
	InputRecordID   int64           `json:"input_record_id"`
	RelationID      string          `json:"relation_id"`
	Language        string          `json:"language"`
	Subject         string          `json:"subject"`
	SubjectEn       string          `json:"subject_en,omitempty"`
	Predicate       string          `json:"predicate"`
	PredicateEn     string          `json:"predicate_en,omitempty"`
	Object          string          `json:"object"`
	ObjectEn        string          `json:"object_en,omitempty"`
	DescText        string          `json:"desc_text"`
	DescTextEn      string          `json:"desc_text_en,omitempty"`
	Keywords        json.RawMessage `json:"keywords"`
	KeywordsEn      json.RawMessage `json:"keywords_en"`
	LineSpans       json.RawMessage `json:"line_spans"`
	Confidence      float64         `json:"confidence"`
	ModelName       string          `json:"model_name"`
	PromptName      string          `json:"prompt_name"`
	SubjectEntityID string          `json:"subject_entity_id,omitempty"`
	ObjectEntityID  string          `json:"object_entity_id,omitempty"`
	Categories      json.RawMessage `json:"categories"`
	SearchDocument  string          `json:"search_document,omitempty"`
	Connected       json.RawMessage `json:"connected_artifacts"`
	ExtInfo         json.RawMessage `json:"ext_info"`
	CreateTime      string          `json:"create_time"`
}

func fetchKnowledgeByArtifactID(db *sql.DB, artifactID string) (knowledgeRecord, int64, error) {
	recordID, _, err := parseArtifactSearchIDSeq(artifactID, "knowledge")
	if err != nil {
		return knowledgeRecord{}, 0, err
	}
	const q = `
SELECT
	id, input_record_id, knowledge_id, COALESCE(language, ''), COALESCE(knowledge_type, ''),
	COALESCE(knowledge_value, ''), COALESCE(knowledge_value_en, ''),
	COALESCE(desc_text, ''), COALESCE(desc_text_en, ''),
	COALESCE(keywords, '[]'::jsonb), COALESCE(keywords_en, '[]'::jsonb),
	COALESCE(lines, '[]'::jsonb), COALESCE(category_paths, '[]'::jsonb), COALESCE(category_paths_en, '[]'::jsonb),
	COALESCE(model_name, ''), COALESCE(prompt_name, ''), COALESCE(search_document, ''),
	COALESCE(to_char(create_time, 'YYYY-MM-DD\"T\"HH24:MI:SSOF'), '')
FROM kb.knowledges
WHERE input_record_id = $1 AND knowledge_id = $2
LIMIT 1`
	var (
		r               knowledgeRecord
		keywords        []byte
		keywordsEn      []byte
		lines           []byte
		categoryPaths   []byte
		categoryPathsEn []byte
	)
	if err := db.QueryRow(q, recordID, artifactID).Scan(
		&r.ID, &r.InputRecordID, &r.KnowledgeID, &r.Language, &r.KnowledgeType,
		&r.KnowledgeValue, &r.KnowledgeValueEn,
		&r.DescText, &r.DescTextEn,
		&keywords, &keywordsEn, &lines, &categoryPaths, &categoryPathsEn,
		&r.ModelName, &r.PromptName, &r.SearchDocument, &r.CreateTime,
	); err != nil {
		return knowledgeRecord{}, 0, err
	}
	r.Keywords = jsonArrayOrEmpty(keywords)
	r.KeywordsEn = jsonArrayOrEmpty(keywordsEn)
	r.Lines = jsonArrayOrEmpty(lines)
	r.CategoryPaths = jsonArrayOrEmpty(categoryPaths)
	r.CategoryPathsEn = jsonArrayOrEmpty(categoryPathsEn)
	return r, r.InputRecordID, nil
}

func fetchEntityByArtifactID(db *sql.DB, artifactID string) (entityRecord, int64, error) {
	recordID, _, err := parseArtifactSearchIDSeq(artifactID, "entity")
	if err != nil {
		return entityRecord{}, 0, err
	}
	const q = `
SELECT
	id, input_record_id, entity_id, COALESCE(language, ''), COALESCE(entity, ''), COALESCE(entity_en, ''),
	COALESCE(entity_type, ''), COALESCE(entity_type_en, ''),
	COALESCE(aliases, '[]'::jsonb), COALESCE(aliases_en, '[]'::jsonb),
	COALESCE(desc_text, ''), COALESCE(desc_text_en, ''),
	COALESCE(keywords, '[]'::jsonb), COALESCE(keywords_en, '[]'::jsonb), COALESCE(line_spans, '[]'::jsonb),
	COALESCE(confidence, 0), COALESCE(model_name, ''), COALESCE(prompt_name, ''),
	COALESCE(entity_context, ''), COALESCE(doc_name, ''), COALESCE(categories, '[]'::jsonb),
	COALESCE(entity_status, ''), COALESCE(search_document, ''), COALESCE(connected_artifacts, '{}'::jsonb),
	COALESCE(ext_info, '{}'::jsonb), COALESCE(to_char(create_time, 'YYYY-MM-DD\"T\"HH24:MI:SSOF'), '')
FROM kb.entities
WHERE input_record_id = $1 AND entity_id = $2
LIMIT 1`
	var (
		r          entityRecord
		aliases    []byte
		aliasesEn  []byte
		keywords   []byte
		keywordsEn []byte
		lineSpans  []byte
		categories []byte
		connected  []byte
		extInfo    []byte
	)
	if err := db.QueryRow(q, recordID, artifactID).Scan(
		&r.ID, &r.InputRecordID, &r.EntityID, &r.Language, &r.Entity, &r.EntityEn,
		&r.EntityType, &r.EntityTypeEn,
		&aliases, &aliasesEn,
		&r.DescText, &r.DescTextEn,
		&keywords, &keywordsEn, &lineSpans,
		&r.Confidence, &r.ModelName, &r.PromptName,
		&r.EntityContext, &r.DocName, &categories,
		&r.EntityStatus, &r.SearchDocument, &connected,
		&extInfo, &r.CreateTime,
	); err != nil {
		return entityRecord{}, 0, err
	}
	r.Aliases = jsonArrayOrEmpty(aliases)
	r.AliasesEn = jsonArrayOrEmpty(aliasesEn)
	r.Keywords = jsonArrayOrEmpty(keywords)
	r.KeywordsEn = jsonArrayOrEmpty(keywordsEn)
	r.LineSpans = jsonArrayOrEmpty(lineSpans)
	r.Categories = jsonArrayOrEmpty(categories)
	r.Connected = jsonArrayOrEmpty(connected)
	r.ExtInfo = jsonArrayOrEmpty(extInfo)
	return r, r.InputRecordID, nil
}

func fetchRelationByArtifactID(db *sql.DB, artifactID string) (relationRecord, int64, error) {
	recordID, _, err := parseArtifactSearchIDSeq(artifactID, "relation")
	if err != nil {
		return relationRecord{}, 0, err
	}
	const q = `
SELECT
	id, input_record_id, relation_id, COALESCE(language, ''), COALESCE(subject, ''), COALESCE(subject_en, ''),
	COALESCE(predicate, ''), COALESCE(predicate_en, ''), COALESCE(object, ''), COALESCE(object_en, ''),
	COALESCE(desc_text, ''), COALESCE(desc_text_en, ''),
	COALESCE(keywords, '[]'::jsonb), COALESCE(keywords_en, '[]'::jsonb), COALESCE(line_spans, '[]'::jsonb),
	COALESCE(confidence, 0), COALESCE(model_name, ''), COALESCE(prompt_name, ''),
	COALESCE(subject_entity_id, ''), COALESCE(object_entity_id, ''), COALESCE(categories, '[]'::jsonb),
	COALESCE(search_document, ''), COALESCE(connected_artifacts, '{}'::jsonb), COALESCE(ext_info, '{}'::jsonb),
	COALESCE(to_char(create_time, 'YYYY-MM-DD\"T\"HH24:MI:SSOF'), '')
FROM kb.relations
WHERE input_record_id = $1 AND relation_id = $2
LIMIT 1`
	var (
		r          relationRecord
		keywords   []byte
		keywordsEn []byte
		lineSpans  []byte
		categories []byte
		connected  []byte
		extInfo    []byte
	)
	if err := db.QueryRow(q, recordID, artifactID).Scan(
		&r.ID, &r.InputRecordID, &r.RelationID, &r.Language, &r.Subject, &r.SubjectEn,
		&r.Predicate, &r.PredicateEn, &r.Object, &r.ObjectEn,
		&r.DescText, &r.DescTextEn,
		&keywords, &keywordsEn, &lineSpans,
		&r.Confidence, &r.ModelName, &r.PromptName,
		&r.SubjectEntityID, &r.ObjectEntityID, &categories,
		&r.SearchDocument, &connected, &extInfo, &r.CreateTime,
	); err != nil {
		return relationRecord{}, 0, err
	}
	r.Keywords = jsonArrayOrEmpty(keywords)
	r.KeywordsEn = jsonArrayOrEmpty(keywordsEn)
	r.LineSpans = jsonArrayOrEmpty(lineSpans)
	r.Categories = jsonArrayOrEmpty(categories)
	r.Connected = jsonArrayOrEmpty(connected)
	r.ExtInfo = jsonArrayOrEmpty(extInfo)
	return r, r.InputRecordID, nil
}

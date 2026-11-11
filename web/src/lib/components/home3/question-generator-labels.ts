import { m } from '$lib/paraglide/messages.js';
import type { QuestionPurpose } from '$lib/services/questionGeneratorService';

// Display labels for question purposes and search artifact types (openspec
// question-generator-artifact-types). Unknown historical values display as stored.

const purposeLabels: Record<QuestionPurpose, () => string> = {
	search_metrics: m.qg_search_metrics,
	search_chunks: m.qg_purpose_search_chunks,
	search_summaries: m.qg_purpose_search_summaries,
	search_topics: m.qg_purpose_search_topics,
	search_scene_blocks: m.qg_purpose_search_scene_blocks,
	search_provisions: m.qg_purpose_search_provisions,
	search_products: m.qg_purpose_search_products,
	search_inventory_items: m.qg_purpose_search_inventory_items,
	search_semantic_projections: m.qg_purpose_search_semantic_projections,
	search_knowledge: m.qg_purpose_search_knowledge,
	search_entities: m.qg_purpose_search_entities,
	search_relations: m.qg_purpose_search_relations,
	search_mixed: m.qg_purpose_search_mixed
};

const artifactTypeLabels: Record<string, () => string> = {
	metric: m.qg_type_metric,
	chunk: m.qg_type_chunk,
	summary: m.qg_type_summary,
	topic: m.qg_type_topic,
	scene_block: m.qg_type_scene_block,
	provision: m.qg_type_provision,
	product: m.qg_type_product,
	inventory_item: m.qg_type_inventory_item,
	semantic_projection: m.qg_type_semantic_projection,
	knowledge: m.qg_type_knowledge,
	entity: m.qg_type_entity,
	relation: m.qg_type_relation
};

export function questionPurposeLabel(purpose: string): string {
	return purposeLabels[purpose as QuestionPurpose]?.() ?? purpose;
}

export function artifactTypeLabel(artifactType: string): string {
	return artifactTypeLabels[artifactType]?.() ?? artifactType;
}

export type KbSearchArtifactType =
	| 'all'
	| 'metrics'
	| 'summaries'
	| 'topics'
	| 'scene-blocks'
	| 'provisions'
	| 'products';

export type KbSearchParams = {
	q: string;
	inputRecordId?: string;
	page?: number;
	pageSize?: number;
	artifactTypes?: string;
	categoryPath?: string;
	topicType?: string;
	sceneType?: string;
	provisionType?: string;
	productType?: string;
	relationType?: string;
};

export type KbSearchResult = {
	artifact_id?: string;
	artifact_type?: string;
	id?: string | number;
	input_record_id?: number;
	inputId?: number;
	primary_label?: string;
	snippet?: string;
	score?: number;
	source_title?: string;
	source_filename?: string;
	secondary_label?: string;
	[key: string]: unknown;
};

export type KbSearchResponse = {
	status: boolean;
	query: string;
	artifact_type?: string;
	page?: number;
	page_size?: number;
	total?: number;
	results?: KbSearchResult[];
	[key: string]: unknown;
};

const endpointByType: Record<KbSearchArtifactType, string> = {
	all: '/api/v1/kb/search',
	metrics: '/api/v1/kb/metrics/search',
	summaries: '/api/v1/kb/summaries/search',
	topics: '/api/v1/kb/topics/search',
	'scene-blocks': '/api/v1/kb/scene-blocks/search',
	provisions: '/api/v1/kb/provisions/search',
	products: '/api/v1/kb/products/search'
};

export async function searchKbArtifacts(
	artifactType: KbSearchArtifactType,
	params: KbSearchParams
): Promise<KbSearchResponse> {
	const search = new URLSearchParams();
	search.set('q', params.q);
	search.set('page', String(params.page ?? 1));
	search.set('page_size', String(params.pageSize ?? 20));
	if (params.inputRecordId?.trim()) {
		search.set('input_record_id', params.inputRecordId.trim());
	}
	if (params.artifactTypes?.trim()) {
		search.set('artifact_types', params.artifactTypes.trim());
	}
	if (params.categoryPath?.trim()) {
		search.set('category_path', params.categoryPath.trim());
	}
	if (params.topicType?.trim()) {
		search.set('topic_type', params.topicType.trim());
	}
	if (params.sceneType?.trim()) {
		search.set('scene_type', params.sceneType.trim());
	}
	if (params.provisionType?.trim()) {
		search.set('provision_type', params.provisionType.trim());
	}
	if (params.productType?.trim()) {
		search.set('product_type', params.productType.trim());
	}
	if (params.relationType?.trim()) {
		search.set('relation_type', params.relationType.trim());
	}

	const res = await fetch(`${endpointByType[artifactType]}?${search.toString()}`, {
		method: 'GET',
		headers: { Accept: 'application/json' }
	});
	const payload = (await res.json()) as KbSearchResponse;
	if (!res.ok) {
		throw new Error(String(payload.error ?? payload.error_msg ?? `HTTP ${res.status}`));
	}
	return payload;
}

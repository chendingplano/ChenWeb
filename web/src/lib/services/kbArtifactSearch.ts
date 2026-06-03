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

const KB_SEARCH_ENDPOINT = '/api/v1/kb/search';

const artifactTypeFilterByScope: Partial<Record<KbSearchArtifactType, string>> = {
	metrics: 'metric',
	summaries: 'summary',
	topics: 'topic',
	'scene-blocks': 'scene_block',
	provisions: 'provision',
	products: 'product'
};

export function buildKbArtifactSearchUrl(
	artifactType: KbSearchArtifactType,
	params: KbSearchParams
): string {
	const search = new URLSearchParams();
	search.set('q', params.q);
	search.set('page', String(params.page ?? 1));
	search.set('page_size', String(params.pageSize ?? 20));
	if (params.inputRecordId?.trim()) {
		search.set('input_record_id', params.inputRecordId.trim());
	}
	if (params.artifactTypes?.trim()) {
		search.set('artifact_types', params.artifactTypes.trim());
	} else {
		const scopedArtifactType = artifactTypeFilterByScope[artifactType];
		if (scopedArtifactType) {
			search.set('artifact_types', scopedArtifactType);
		}
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

	return `${KB_SEARCH_ENDPOINT}?${search.toString()}`;
}

export async function searchKbArtifacts(
	artifactType: KbSearchArtifactType,
	params: KbSearchParams
): Promise<KbSearchResponse> {
	// A search with no matches is a valid outcome, and the backend (or a proxy in
	// front of it) may answer with a dropped connection or an empty body rather
	// than a JSON envelope. Treat those as "zero results" instead of surfacing a
	// raw network error. Genuine HTTP error statuses that carry a body still throw.
	const emptyResponse: KbSearchResponse = {
		status: true,
		query: params.q,
		page: params.page ?? 1,
		page_size: params.pageSize ?? 20,
		total: 0,
		results: []
	};

	let res: Response;
	try {
		res = await fetch(buildKbArtifactSearchUrl(artifactType, params), {
			method: 'GET',
			headers: { Accept: 'application/json' }
		});
	} catch {
		// Network-level failure (e.g. ERR_EMPTY_RESPONSE): the connection closed
		// before any response. Acceptable here — report it as no results.
		return emptyResponse;
	}

	const body = (await res.text()).trim();
	if (!body) {
		// Empty body (including 204 / dropped payload) — also no results.
		return emptyResponse;
	}

	const payload = JSON.parse(body) as KbSearchResponse;
	if (!res.ok) {
		throw new Error(String(payload.error ?? payload.error_msg ?? `HTTP ${res.status}`));
	}
	return payload;
}

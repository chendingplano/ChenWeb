// Service for the per-metric wiki page. Fetches the cached page JSON from the
// backend, which lazily generates it on first request. See the backend handler
// GET /api/v1/kb/metrics/:metric_id/wiki.

export type MetricWikiDocMeta = {
	record_id: number;
	title: string;
	file_name: string;
	type: string;
};

export type MetricWikiInfobox = {
	value?: string;
	unit?: string;
	range_type?: string;
	threshold_or_target?: string;
	measurement_frequency?: string;
	subject?: string;
	confidence?: number;
};

export type MetricWikiCorpus = {
	source_document: MetricWikiDocMeta;
	source_excerpt?: string;
	chunk_summary?: string;
};

export type MetricWikiGenerated = {
	model: string;
	lang: string;
	schema_version: number;
	source_hash: string;
};

export type MetricWikiPage = {
	metric_id: string;
	title: string;
	lead: string;
	infobox: MetricWikiInfobox;
	definition?: string;
	background?: string;
	how_used?: string;
	choosing_values?: string;
	in_this_corpus: MetricWikiCorpus;
	related_metrics?: string[];
	generated: MetricWikiGenerated;
};

export type MetricWikiResponse = {
	status: boolean;
	generated: boolean;
	page: MetricWikiPage;
	error?: string;
	error_msg?: string;
};

// metricIdFromArtifactId returns the canonical metric_id
// ("<record_id>_mtc_<seqno>") from a metric search-artifact id. Returns null
// when the value is not a metric artifact id.
export function metricIdFromArtifactId(artifactId: string | undefined | null): string | null {
	if (!artifactId) return null;
	const idx = artifactId.indexOf('_mtc_');
	if (idx <= 0) return null;
	return artifactId;
}

// getMetricWiki fetches (and, on a cache miss, triggers generation of) a metric
// wiki page. The first request for a missing page can be slow while the backend
// generates it.
export async function getMetricWiki(metricId: string, lang?: string): Promise<MetricWikiResponse> {
	const langQuery = lang ? `?lang=${encodeURIComponent(lang)}` : '';
	const url = `/api/v1/kb/metrics/${encodeURIComponent(metricId)}/wiki${langQuery}`;

	const res = await fetch(url, { method: 'GET', headers: { Accept: 'application/json' } });
	const body = (await res.text()).trim();
	if (!body) {
		throw new Error('empty response from server');
	}

	const payload = JSON.parse(body) as MetricWikiResponse;
	if (!res.ok || !payload.status) {
		throw new Error(String(payload.error ?? payload.error_msg ?? `HTTP ${res.status}`));
	}
	return payload;
}

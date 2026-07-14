export type ArtifactWikiGeneratedMeta = {
	model: string;
	lang: string;
	schema_version: number;
	source_hash: string;
};

export type ArtifactWikiResponse = {
	status: boolean;
	fresh: boolean;
	artifact_type: string;
	artifact_id: string;
	article: Record<string, unknown>;
	record: Record<string, unknown>;
	source_document?: Record<string, unknown>;
	generated: ArtifactWikiGeneratedMeta;
	error?: string;
	error_msg?: string;
};

export type BuildArtifactWikiHrefParams = {
	artifactType: string;
	artifactId: string;
	lang?: string;
};

export type BuildArtifactWikiApiUrlParams = {
	artifactType: string;
	artifactId: string;
	lang?: string;
	includeArticle?: boolean;
};

export function buildArtifactWikiHref({
	artifactType,
	artifactId,
	lang = 'en'
}: BuildArtifactWikiHrefParams): string {
	const params = new URLSearchParams({
		section: 'kb-artifact-wiki',
		artifact_type: artifactType,
		artifact_id: artifactId,
		lang
	});
	return `/home3/knowledge?${params.toString()}`;
}

export function buildArtifactWikiApiUrl({
	artifactType,
	artifactId,
	lang,
	includeArticle = true
}: BuildArtifactWikiApiUrlParams): string {
	const query = new URLSearchParams({
		artifact_type: artifactType,
		artifact_id: artifactId
	});
	if (lang?.trim()) query.set('lang', lang.trim());
	if (!includeArticle) query.set('include_article', '0');
	return `/api/v1/kb/artifacts/wiki?${query.toString()}`;
}

async function fetchArtifactWiki(url: string): Promise<ArtifactWikiResponse> {
	const res = await fetch(url, {
		method: 'GET',
		headers: { Accept: 'application/json' }
	});
	const body = (await res.text()).trim();
	if (!body) throw new Error('empty response from server');
	const payload = JSON.parse(body) as ArtifactWikiResponse;
	if (!res.ok || !payload.status) {
		throw new Error(String(payload.error ?? payload.error_msg ?? `HTTP ${res.status}`));
	}
	return payload;
}

export async function getArtifactWiki(
	artifactType: string,
	artifactId: string,
	lang?: string
): Promise<ArtifactWikiResponse> {
	return fetchArtifactWiki(
		buildArtifactWikiApiUrl({ artifactType, artifactId, lang, includeArticle: true })
	);
}

export async function getArtifactWikiRecord(
	artifactType: string,
	artifactId: string,
	lang?: string
): Promise<ArtifactWikiResponse> {
	return fetchArtifactWiki(
		buildArtifactWikiApiUrl({ artifactType, artifactId, lang, includeArticle: false })
	);
}

// kb.doc_review_findings.artifact_id / kb.doc_review_logs.unit_key encode the
// artifact type in their business-key format: "<record_id>_<mtc|prv|inv>_<seqno>".
const artifactPattern = /^[1-9][0-9]*_(mtc|prv|inv)_[1-9][0-9]*$/;

const ARTIFACT_TYPE_BY_SUFFIX = { mtc: 'metric', prv: 'provision', inv: 'inventory_item' } as const;

// Returns the artifact-wiki type ('metric' | 'provision' | 'inventory_item')
// encoded in an artifact key, or null when the key doesn't match the pattern.
export function artifactTypeFromKey(key: string): string | null {
	const match = key.match(artifactPattern);
	if (!match) return null;
	return ARTIFACT_TYPE_BY_SUFFIX[match[1] as keyof typeof ARTIFACT_TYPE_BY_SUFFIX];
}

// Fetches an artifact's full record via the artifact-wiki endpoint.
export async function getArtifactWiki(artifactType: string, artifactId: string): Promise<Record<string, unknown>> {
	const params = new URLSearchParams({ artifact_type: artifactType, artifact_id: artifactId, include_article: '0' });
	const res = await fetch(`/api/v1/kb/artifacts/wiki?${params}`, { credentials: 'same-origin' });
	const body = await res.text();
	let data: Record<string, unknown> = {};
	try {
		data = JSON.parse(body);
	} catch {
		if (!res.ok) throw new Error(body || 'Failed to load artifact');
		throw new Error('Artifact response was not valid JSON');
	}
	if (!res.ok || !data.record) throw new Error(String(data.error_msg ?? data.message ?? 'Failed to load artifact'));
	return data.record as Record<string, unknown>;
}

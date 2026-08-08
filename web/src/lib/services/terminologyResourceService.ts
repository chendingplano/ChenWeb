// Client for the External Terminology Resources admin API (System Admin >
// Resources). Mirrors server/api/terminologyresourcehandler resourceView.
export type TerminologyResource = {
	id: string;
	name: string;
	description: string;
	url: string;
	release: string;
	license: string;
	license_url: string;
	can_download: boolean;
	permission_required: boolean;
	notes: string;
	downloaded: boolean;
	downloaded_at: string | null;
	/** True while the server is streaming a (re-)download for this source. */
	downloading: boolean;
	downloaded_bytes: number;
	total_bytes: number;
	sha256: string;
	size_bytes: number;
	/** Typical artifact size in bytes before download; 0 = varies. */
	expected_size_bytes: number;
	/** Hard server-side size cap enforced during fetch. */
	max_bytes: number;
	/** Upstream update cadence ("weekly" for Wikidata); empty = pinned. */
	update_cadence: string;
	artifact: string;
	source_url: string;
	manifest_draft: string;
	/** Draft manifest license_review_status: "pending_review", "approved",
	 * "disapproved", or "" when there is no draft. */
	review_status: string;
	review_comments: string;
	reviewed_by: string;
	reviewed_at: string | null;
	error: string;
};

type ListResponse = {
	status: boolean;
	resources: TerminologyResource[];
};

type DownloadResponse = {
	status: boolean;
	resource: TerminologyResource;
};

export type ImportCounts = {
	entries: number;
	labels: number;
	relations: number;
	negative_decisions: number;
	ucum_codes: number;
	artifacts: number;
};

export type ImportResult = {
	source: string;
	release: string;
	counts: ImportCounts;
	replayed: boolean;
};

/** Outcome of the offline import started after an approval. ok is true only
 * when the import committed; error explains failures (approval still stands). */
export type ImportOutcome = {
	ok: boolean;
	result?: ImportResult;
	error?: string;
};

type ApproveResponse = {
	status: boolean;
	resource: TerminologyResource;
	import: ImportOutcome;
};

function encodeForm(fields: Record<string, string>): string {
	const params = new URLSearchParams();
	for (const [key, value] of Object.entries(fields)) {
		if (value) params.set(key, value);
	}
	return params.toString();
}

async function getJSON<T>(url: string, fetchFn: typeof fetch): Promise<T> {
	const res = await fetchFn(url, { credentials: 'same-origin' });
	if (!res.ok) {
		const text = await res.text();
		throw new Error(text || `request failed: ${res.status}`);
	}
	return (await res.json()) as T;
}

/** Every portfolio resource with its persisted download status. */
export function listTerminologyResources(
	fetchFn: typeof fetch = fetch
): Promise<TerminologyResource[]> {
	return getJSON<ListResponse>('/api/v1/terminology-resources', fetchFn).then(
		(d) => d.resources ?? []
	);
}

/** Download one freely available resource; returns its updated status. */
export async function downloadTerminologyResource(
	source: string,
	fetchFn: typeof fetch = fetch
): Promise<TerminologyResource> {
	const res = await fetchFn(
		`/api/v1/terminology-resources/${encodeURIComponent(source)}/download`,
		{
			method: 'POST',
			credentials: 'same-origin'
		}
	);
	let body: DownloadResponse;
	try {
		body = (await res.json()) as DownloadResponse;
	} catch {
		throw new Error(`download failed: ${res.status}`);
	}
	if (!res.ok || !body.status) {
		throw new Error(
			body?.resource?.error ??
				(body as { error?: string })?.error ??
				`download failed: ${res.status}`
		);
	}
	return body.resource;
}

/** Mark one pending draft manifest as license-reviewed and approved, then
 * start importing it. The server records approved_by as the authenticated
 * user unless overridden, plus the operator's review comments. Returns the
 * updated resource and the import outcome. */
export async function approveTerminologyResource(
	source: string,
	opts: { approvedBy?: string; comments?: string; fetchFn?: typeof fetch } = {}
): Promise<{ resource: TerminologyResource; import: ImportOutcome }> {
	const { approvedBy, comments, fetchFn = fetch } = opts;
	const body = encodeForm({ approved_by: approvedBy ?? '', comments: comments ?? '' });
	const res = await fetchFn(`/api/v1/terminology-resources/${encodeURIComponent(source)}/approve`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: body ? { 'Content-Type': 'application/x-www-form-urlencoded' } : undefined,
		body: body || undefined
	});
	let data: ApproveResponse;
	try {
		data = (await res.json()) as ApproveResponse;
	} catch {
		throw new Error(`approve failed: ${res.status}`);
	}
	if (!res.ok || !data.status) {
		throw new Error((data as { error?: string })?.error ?? `approve failed: ${res.status}`);
	}
	return { resource: data.resource, import: data.import };
}

/** Record an operator rejection of one pending draft manifest. The server
 * saves the comments and reviewer, marks the draft disapproved, and never
 * imports it. */
export async function disapproveTerminologyResource(
	source: string,
	opts: { reviewedBy?: string; comments?: string; fetchFn?: typeof fetch } = {}
): Promise<TerminologyResource> {
	const { reviewedBy, comments, fetchFn = fetch } = opts;
	const body = encodeForm({ reviewed_by: reviewedBy ?? '', comments: comments ?? '' });
	const res = await fetchFn(
		`/api/v1/terminology-resources/${encodeURIComponent(source)}/disapprove`,
		{
			method: 'POST',
			credentials: 'same-origin',
			headers: body ? { 'Content-Type': 'application/x-www-form-urlencoded' } : undefined,
			body: body || undefined
		}
	);
	let data: DownloadResponse;
	try {
		data = (await res.json()) as DownloadResponse;
	} catch {
		throw new Error(`disapprove failed: ${res.status}`);
	}
	if (!res.ok || !data.status) {
		throw new Error((data as { error?: string })?.error ?? `disapprove failed: ${res.status}`);
	}
	return data.resource;
}

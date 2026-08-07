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
	sha256: string;
	size_bytes: number;
	artifact: string;
	source_url: string;
	manifest_draft: string;
	/** Draft manifest license_review_status: "pending_review", "approved", or "". */
	review_status: string;
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

/** Mark one pending draft manifest as license-reviewed and approved. The
 * server records approved_by as the authenticated user unless overridden. */
export async function approveTerminologyResource(
	source: string,
	approvedBy?: string,
	fetchFn: typeof fetch = fetch
): Promise<TerminologyResource> {
	const body = approvedBy ? `approved_by=${encodeURIComponent(approvedBy)}` : '';
	const res = await fetchFn(`/api/v1/terminology-resources/${encodeURIComponent(source)}/approve`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: body ? { 'Content-Type': 'application/x-www-form-urlencoded' } : undefined,
		body: body || undefined
	});
	let data: DownloadResponse;
	try {
		data = (await res.json()) as DownloadResponse;
	} catch {
		throw new Error(`approve failed: ${res.status}`);
	}
	if (!res.ok || !data.status) {
		throw new Error((data as { error?: string })?.error ?? `approve failed: ${res.status}`);
	}
	return data.resource;
}

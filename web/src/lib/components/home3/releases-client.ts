export type ReleaseItem = {
	id?: number;
	item_type: 'bug fix' | 'improvement' | 'new feature';
	description: string;
	notes: string;
	pull_request: string;
	ticket_num: string;
};

export type ReleaseInput = {
	major_version: string;
	minor_version: string;
	release_notes: string;
	release_date: string;
	items: ReleaseItem[];
};

export type Release = ReleaseInput & { id: number };

type Envelope = { status: boolean; results?: Release[]; error_msg?: string };

async function request(url: string, init?: RequestInit): Promise<Envelope> {
	const response = await fetch(url, { credentials: 'same-origin', ...init });
	const body = await response.text();
	let parsed: Envelope | null = null;
	try {
		parsed = body ? (JSON.parse(body) as Envelope) : null;
	} catch {
		/* use HTTP status below */
	}
	if (!response.ok) throw new Error(parsed?.error_msg || `Request failed (${response.status})`);
	return parsed ?? { status: true };
}

export async function listReleases(): Promise<Release[]> {
	return (await request('/api/v1/releases')).results ?? [];
}

export function createRelease(input: ReleaseInput): Promise<Envelope> {
	return request('/api/v1/releases', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
	});
}

export function updateRelease(id: number, input: ReleaseInput): Promise<Envelope> {
	return request(`/api/v1/releases/${id}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
	});
}

export function deleteRelease(id: number): Promise<Envelope> {
	return request(`/api/v1/releases/${id}`, { method: 'DELETE' });
}

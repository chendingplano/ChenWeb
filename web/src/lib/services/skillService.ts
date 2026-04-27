const BASE = '/api/v1/kb';

export type SkillRecord = {
	id: number;
	name: string;
	description?: string;
	category: string[];
	status?: string;
	tags: string[];
	version?: string;
	notes?: string;
	public_info?: unknown;
	create_time: string;
	modify_time: string;
};

export type SkillCategoryRecord = {
	id: number;
	name: string;
	level: number;
	parent_id?: number;
	path: string[];
	description?: string;
};

export type ListSkillsParams = {
	category?: string[];
	status?: string;
	page?: number;
	pageSize?: number;
};

export type ListSkillsResponse = {
	status: boolean;
	results: SkillRecord[];
	total: number;
	page: number;
	page_size: number;
};

export type ListCategoriesResponse = {
	status: boolean;
	results: SkillCategoryRecord[];
	total: number;
};

export type UpdateSkillPayload = {
	name: string;
	description?: string | null;
	category: string[];
	status?: string | null;
	tags: string[];
	version?: string | null;
	notes?: string | null;
};

export async function listSkillCategories(): Promise<ListCategoriesResponse> {
	const res = await fetch(`${BASE}/skill-categories`, { credentials: 'same-origin' });
	if (!res.ok) throw new Error(`Failed to load skill categories: ${res.status}`);
	return res.json();
}

export async function listSkills(params: ListSkillsParams = {}): Promise<ListSkillsResponse> {
	const q = new URLSearchParams();
	if (params.category && params.category.length > 0) {
		q.set('category', JSON.stringify(params.category));
	}
	if (params.status) q.set('status', params.status);
	if (params.page) q.set('page', String(params.page));
	if (params.pageSize) q.set('page_size', String(params.pageSize));

	const res = await fetch(`${BASE}/skills?${q}`, { credentials: 'same-origin' });
	if (!res.ok) throw new Error(`Failed to load skills: ${res.status}`);
	return res.json();
}

export async function updateSkill(id: number, payload: UpdateSkillPayload): Promise<void> {
	const res = await fetch(`${BASE}/skills/${id}`, {
		method: 'PUT',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!res.ok) {
		const body = await res.json().catch(() => ({}));
		throw new Error(body.error_msg ?? `Failed to update skill: ${res.status}`);
	}
}

export async function deleteSkill(id: number): Promise<void> {
	const res = await fetch(`${BASE}/skills/${id}`, {
		method: 'DELETE',
		credentials: 'same-origin'
	});
	if (!res.ok) {
		const body = await res.json().catch(() => ({}));
		throw new Error(body.error_msg ?? `Failed to delete skill: ${res.status}`);
	}
}

export type CreateSkillPayload = {
	name: string;
	description?: string | null;
	category: string[];
	status?: string | null;
	tags: string[];
	version?: string | null;
	notes?: string | null;
};

export async function createSkill(payload: CreateSkillPayload): Promise<SkillRecord> {
	const res = await fetch(`${BASE}/skills`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!res.ok) {
		const body = await res.json().catch(() => ({}));
		throw new Error(body.error_msg ?? `Failed to create skill: ${res.status}`);
	}
	return res.json();
}

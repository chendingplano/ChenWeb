export type ManagedUser = {
	id: string;
	name: string;
	first_name: string;
	last_name: string;
	email: string;
	verified: boolean;
	admin: boolean;
	is_owner: boolean;
	roles: string[];
	user_status: string;
	created: string;
	updated: string;
};

type ListUsersResponse = {
	status: string;
	users?: ManagedUser[];
};

async function req<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(path, { credentials: 'same-origin', ...init });
	const text = await res.text();
	let parsed: unknown = null;
	if (text) {
		try {
			parsed = JSON.parse(text);
		} catch {
			parsed = null;
		}
	}
	if (!res.ok) {
		const message =
			parsed && typeof parsed === 'object' && parsed !== null && 'error' in parsed
				? String((parsed as { error: unknown }).error)
				: `HTTP ${res.status}`;
		throw new Error(message);
	}
	return parsed as T;
}

export async function listManagedUsers(): Promise<ManagedUser[]> {
	const response = await req<ListUsersResponse>('/api/v1/system-admin/users');
	return response.users ?? [];
}

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

export type ManagedRole = {
	key: string;
	label: string;
	description: string;
	status: string;
};

type ListUsersResponse = {
	status: string;
	users?: ManagedUser[];
};

type ListRolesResponse = {
	status: string;
	roles?: ManagedRole[];
};

type ManagedUserResponse = {
	status: string;
	user: ManagedUser;
};

export type UpdateManagedUserInput = {
	first_name: string;
	last_name: string;
	status: 'active' | 'inactive' | 'trial';
	admin: boolean;
	roles: string[];
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

export async function listManagedRoles(): Promise<ManagedRole[]> {
	const response = await req<ListRolesResponse>('/api/v1/system-admin/roles');
	return response.roles ?? [];
}

export async function updateManagedUser(email: string, input: UpdateManagedUserInput): Promise<ManagedUser> {
	const response = await req<ManagedUserResponse>(
		`/api/v1/system-admin/users/by-email/${encodeURIComponent(email)}`,
		{
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
		}
	);
	return response.user;
}

export async function deleteManagedUser(email: string): Promise<void> {
	await req<{ status: string; deleted: boolean }>(
		`/api/v1/system-admin/users/by-email/${encodeURIComponent(email)}`,
		{
			method: 'DELETE'
		}
	);
}

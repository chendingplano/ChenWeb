export type BuildProcess = {
	executable: string;
	status: 'running' | 'stopped' | 'unreported';
	pid?: number;
	build_number?: string;
	build_name?: string;
	target_machine?: string;
	os_info?: string;
	description?: string;
	started_at?: string;
};

type ListResponse = {
	status: boolean;
	processes?: BuildProcess[];
	error_msg?: string;
};

export async function listBuildProcesses(fetchFn: typeof fetch = fetch): Promise<BuildProcess[]> {
	const response = await fetchFn('/api/v1/system-admin/build-info', { credentials: 'same-origin' });
	const body = (await response.json()) as ListResponse;
	if (!response.ok || !body.status) {
		throw new Error(body.error_msg || `Request failed (${response.status})`);
	}
	return body.processes ?? [];
}

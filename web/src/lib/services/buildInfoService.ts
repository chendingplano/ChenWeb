export type BuildActivity = {
	id: number;
	build_number: string;
	build_name: string;
	executable: string;
	target_machine: string;
	os_info: string;
	description: string;
	activity: string;
	created_at: string;
};

type ListResponse = {
	status: boolean;
	results?: BuildActivity[];
	error_msg?: string;
};

export async function listBuildActivities(fetchFn: typeof fetch = fetch): Promise<BuildActivity[]> {
	const response = await fetchFn('/api/v1/system-admin/build-info', { credentials: 'same-origin' });
	const body = (await response.json()) as ListResponse;
	if (!response.ok || !body.status) {
		throw new Error(body.error_msg || `Request failed (${response.status})`);
	}
	return body.results ?? [];
}

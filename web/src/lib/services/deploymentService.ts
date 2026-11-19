// Client for the Deployment page (System Admin > System > Deployment).
const BASE = '/api/v1/system-admin/deployment';

export type DeploymentRole = 'build' | 'deploy' | '';
export type RunKind = 'build' | 'deploy';
export type StepStatus = 'pending' | 'running' | 'succeeded' | 'failed' | 'skipped';
export type RunStatus = 'queued' | 'running' | 'succeeded' | 'failed';

export type DeploymentStep = {
	seq: number;
	key: string;
	arg: string;
	status: StepStatus;
	started_at?: string;
	finished_at?: string;
	output: string;
	output_truncated: boolean;
};

export type DeploymentRun = {
	id: number;
	kind: RunKind;
	build_number: string;
	build_name?: string;
	description?: string;
	archive_name: string;
	previous_build_number?: string;
	release_version?: string;
	previous_release_version?: string;
	status: RunStatus;
	error_message: string;
	requested_by: string;
	runner_pid: number;
	requested_at?: string;
	started_at?: string;
	finished_at?: string;
	steps?: DeploymentStep[];
};

export type DeploymentStatus = {
	role: DeploymentRole;
	kind: RunKind | '';
	active: DeploymentRun | null;
	recent: DeploymentRun[];
};

/** One archive on the relay; release fields are empty for archives without a release file. */
export type Archive = {
	archive_name: string;
	size: number;
	modified: string;
	build_number: string;
	release_version: string;
	release_date: string;
	item_count: number;
	running: boolean;
};

export type ArchiveList = {
	archives: Archive[];
	running_release: string;
	running_build_number: string;
};

export type DeploymentProcess = {
	id: string;
	unit?: string;
	log_source: string;
	active_state?: string;
	sub_state?: string;
	pid?: number;
	uptime_seconds?: number;
	build_number?: string;
	build_status?: 'running' | 'unreported';
	can_restart: boolean;
};

/** An API error; `code` is one the page translates, `status` 0 means the server did not answer. */
export class DeploymentError extends Error {
	constructor(
		message: string,
		public code: string,
		public status: number
	) {
		super(message);
	}
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	let response: Response;
	try {
		response = await fetch(BASE + path, { credentials: 'same-origin', ...init });
	} catch (err) {
		throw new DeploymentError(err instanceof Error ? err.message : String(err), 'unreachable', 0);
	}
	let body: Record<string, unknown> = {};
	try {
		body = await response.json();
	} catch {
		// A proxy error page during a restart is not JSON.
	}
	if (!response.ok || body.status === false) {
		const unreachable = response.status === 502 || response.status === 503 || response.status === 504;
		throw new DeploymentError(
			String(body.error_msg ?? `HTTP ${response.status}`),
			String(body.code ?? (unreachable ? 'unreachable' : 'internal')),
			unreachable && !body.code ? 0 : response.status
		);
	}
	return body as T;
}

const jsonPost = (body: unknown): RequestInit => ({
	method: 'POST',
	headers: { 'Content-Type': 'application/json' },
	body: JSON.stringify(body)
});

export const getDeploymentStatus = () => request<DeploymentStatus>('/status');

export async function getRun(kind: RunKind, id: number): Promise<DeploymentRun> {
	const body = await request<{ run: DeploymentRun }>(`/${kind === 'build' ? 'builds' : 'deployments'}/${id}`);
	return body.run;
}

export const startBuild = (description: string) =>
	request<{ id: number; build_number: string }>('/builds', jsonPost({ description }));

export const listArchives = () => request<ArchiveList>('/archives');

export const startDeployment = (archiveName: string) =>
	request<{ id: number }>('/deployments', jsonPost({ archive_name: archiveName }));

export async function listProcesses(): Promise<DeploymentProcess[]> {
	const body = await request<{ processes: DeploymentProcess[] }>('/processes');
	return body.processes ?? [];
}

export const restartProcess = (id: string) =>
	request<{ self: boolean }>(`/processes/${encodeURIComponent(id)}/restart`, { method: 'POST' });

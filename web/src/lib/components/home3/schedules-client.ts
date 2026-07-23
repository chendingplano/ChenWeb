export type JobType = {
	job_type: string;
	label: string;
};

export type Schedule = {
	id: number;
	name: string;
	job_type: string;
	interval_seconds: number;
	params: Record<string, unknown>;
	enabled: boolean;
	next_run_at: string;
	last_run_at?: string;
	last_run_status?: string;
};

export type ScheduleRun = {
	id: number;
	schedule_id: number;
	job_type: string;
	status: 'running' | 'success' | 'failed';
	started_at: string;
	finished_at?: string;
	result: Record<string, unknown>;
	error?: string;
};

export type CreateScheduleInput = {
	name: string;
	job_type: string;
	interval_seconds: number;
	params: Record<string, unknown>;
	enabled: boolean;
};

export type UpdateScheduleInput = {
	name: string;
	interval_seconds: number;
	params: Record<string, unknown>;
	enabled: boolean;
};

async function req<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(path, {
		credentials: 'same-origin',
		...init
	});
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
		const msg =
			parsed && typeof parsed === 'object' && parsed !== null && 'error_msg' in parsed
				? String((parsed as { error_msg: unknown }).error_msg)
				: `HTTP ${res.status}`;
		throw new Error(msg);
	}
	return parsed as T;
}

export async function listJobTypes(): Promise<JobType[]> {
	const res = await req<{ job_types: JobType[] }>('/api/v1/kb/schedule-job-types');
	return res.job_types;
}

export async function listSchedules(): Promise<Schedule[]> {
	const res = await req<{ schedules: Schedule[] }>('/api/v1/kb/schedules');
	return res.schedules;
}

export async function createSchedule(input: CreateScheduleInput): Promise<Schedule> {
	const res = await req<{ schedule: Schedule }>('/api/v1/kb/schedules', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
	});
	return res.schedule;
}

export function updateSchedule(id: number, input: UpdateScheduleInput): Promise<void> {
	return req<void>(`/api/v1/kb/schedules/${id}`, {
		method: 'PATCH',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
	});
}

export function deleteSchedule(id: number): Promise<void> {
	return req<void>(`/api/v1/kb/schedules/${id}`, { method: 'DELETE' });
}

export async function listScheduleRuns(id: number, limit = 50): Promise<ScheduleRun[]> {
	const res = await req<{ runs: ScheduleRun[] }>(`/api/v1/kb/schedules/${id}/runs?limit=${limit}`);
	return res.runs;
}

/** Human-readable interval label for the "add schedule" form and schedule cards. */
export function formatInterval(seconds: number): string {
	if (seconds % 86400 === 0 && seconds >= 86400) {
		const days = seconds / 86400;
		return `every ${days} day${days === 1 ? '' : 's'}`;
	}
	if (seconds % 3600 === 0 && seconds >= 3600) {
		const hours = seconds / 3600;
		return `every ${hours} hour${hours === 1 ? '' : 's'}`;
	}
	if (seconds % 60 === 0 && seconds >= 60) {
		const minutes = seconds / 60;
		return `every ${minutes} minute${minutes === 1 ? '' : 's'}`;
	}
	return `every ${seconds}s`;
}

/**
 * Fraction of the interval elapsed since the last run (0 = just ran, 1 = due
 * now / overdue), for the "active schedules" progress-bar view. Clamped to
 * [0, 1]; a schedule with no last_run_at reads as fully due (1).
 */
export function progressToNextRun(schedule: Schedule, now: Date = new Date()): number {
	if (!schedule.last_run_at) {
		return 1;
	}
	const last = new Date(schedule.last_run_at).getTime();
	const elapsedMs = now.getTime() - last;
	const intervalMs = schedule.interval_seconds * 1000;
	if (intervalMs <= 0) {
		return 1;
	}
	return Math.min(1, Math.max(0, elapsedMs / intervalMs));
}

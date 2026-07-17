// Workspace status lists — announcements, recent activities, alarms/errors.
// Shapes mirror server/api/workspacelists/*.go JSON tags exactly.

export interface Announcement {
	group_id: number;
	occurred_at: string;
	importance: string;
	text: string;
}

export interface AnnouncementAdmin {
	group_id: number;
	occurred_at: string;
	importance: string;
	translations: Record<string, string>;
}

export interface RecentActivity {
	group_id: number;
	occurred_at: string;
	activity_type: string;
	text: string;
}

export interface RecentActivityAdmin {
	group_id: number;
	occurred_at: string;
	activity_type: string;
	translations: Record<string, string>;
}

export interface AlarmNote {
	time: string;
	user: string;
	note: string;
}

export interface Alarm {
	id: number;
	occurred_at: string;
	severity: string;
	message: string;
	status: 'unsolved' | 'solved';
	notes: AlarmNote[];
}

export interface TranslatedListInput {
	occurred_at?: string;
	translations: Record<string, string>;
}

export interface AnnouncementInput extends TranslatedListInput {
	importance?: string;
}

export interface ActivityInput extends TranslatedListInput {
	activity_type?: string;
}

async function getJSON<T>(url: string, fetchFn: typeof fetch): Promise<T> {
	const res = await fetchFn(url, { credentials: 'same-origin' });
	if (!res.ok) {
		throw new Error(`workspace lists request failed: ${res.status} ${url}`);
	}
	return (await res.json()) as T;
}

async function sendJSON<T>(
	url: string,
	method: string,
	body: unknown,
	fetchFn: typeof fetch
): Promise<T> {
	const res = await fetchFn(url, {
		method,
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body ?? {})
	});
	if (!res.ok) {
		const payload = await res.json().catch(() => null);
		throw new Error(payload?.error ?? `workspace lists request failed: ${res.status} ${url}`);
	}
	return (await res.json()) as T;
}

// --- Announcements ---

export async function fetchAnnouncements(
	lang: string,
	fetchFn: typeof fetch = fetch
): Promise<Announcement[]> {
	const r = await getJSON<{ announcements: Announcement[] }>(
		`/api/v1/workspace/announcements?lang=${encodeURIComponent(lang)}`,
		fetchFn
	);
	return r.announcements ?? [];
}

export async function fetchAnnouncementsAdmin(
	fetchFn: typeof fetch = fetch
): Promise<AnnouncementAdmin[]> {
	const r = await getJSON<{ announcements: AnnouncementAdmin[] }>(
		'/api/v1/workspace/announcements/admin',
		fetchFn
	);
	return r.announcements ?? [];
}

export function createAnnouncement(
	input: AnnouncementInput,
	fetchFn: typeof fetch = fetch
): Promise<{ group_id: number }> {
	return sendJSON('/api/v1/workspace/announcements', 'POST', input, fetchFn);
}

export function updateAnnouncement(
	groupId: number,
	input: AnnouncementInput,
	fetchFn: typeof fetch = fetch
): Promise<{ group_id: number }> {
	return sendJSON(`/api/v1/workspace/announcements/${groupId}`, 'PUT', input, fetchFn);
}

export function deleteAnnouncement(
	groupId: number,
	fetchFn: typeof fetch = fetch
): Promise<{ deleted: boolean }> {
	return sendJSON(`/api/v1/workspace/announcements/${groupId}`, 'DELETE', {}, fetchFn);
}

// --- Recent activities ---

export async function fetchRecentActivities(
	lang: string,
	fetchFn: typeof fetch = fetch
): Promise<RecentActivity[]> {
	const r = await getJSON<{ recent_activities: RecentActivity[] }>(
		`/api/v1/workspace/recent-activities?lang=${encodeURIComponent(lang)}`,
		fetchFn
	);
	return r.recent_activities ?? [];
}

export async function fetchRecentActivitiesAdmin(
	fetchFn: typeof fetch = fetch
): Promise<RecentActivityAdmin[]> {
	const r = await getJSON<{ recent_activities: RecentActivityAdmin[] }>(
		'/api/v1/workspace/recent-activities/admin',
		fetchFn
	);
	return r.recent_activities ?? [];
}

export function createActivity(
	input: ActivityInput,
	fetchFn: typeof fetch = fetch
): Promise<{ group_id: number }> {
	return sendJSON('/api/v1/workspace/recent-activities', 'POST', input, fetchFn);
}

export function updateActivity(
	groupId: number,
	input: ActivityInput,
	fetchFn: typeof fetch = fetch
): Promise<{ group_id: number }> {
	return sendJSON(`/api/v1/workspace/recent-activities/${groupId}`, 'PUT', input, fetchFn);
}

export function deleteActivity(
	groupId: number,
	fetchFn: typeof fetch = fetch
): Promise<{ deleted: boolean }> {
	return sendJSON(`/api/v1/workspace/recent-activities/${groupId}`, 'DELETE', {}, fetchFn);
}

// --- Alarms / errors (no i18n) ---

export async function fetchAlarms(
	opts: { unsolvedOnly?: boolean; limit?: number } = {},
	fetchFn: typeof fetch = fetch
): Promise<Alarm[]> {
	const params = new URLSearchParams();
	if (opts.unsolvedOnly) params.set('unsolved_only', 'true');
	if (opts.limit) params.set('limit', String(opts.limit));
	const qs = params.toString();
	const r = await getJSON<{ alarms: Alarm[] }>(
		`/api/v1/workspace/alarms${qs ? `?${qs}` : ''}`,
		fetchFn
	);
	return r.alarms ?? [];
}

export function updateAlarm(
	id: number,
	patch: { status?: string; note?: string },
	fetchFn: typeof fetch = fetch
): Promise<{ updated: boolean }> {
	return sendJSON(`/api/v1/workspace/alarms/${id}`, 'PATCH', patch, fetchFn);
}

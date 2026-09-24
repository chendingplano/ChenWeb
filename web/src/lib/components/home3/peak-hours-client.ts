export type ApplicableDaysMode = 'workdays' | 'weekdays' | 'days_of_month';

export type ApplicableDays = {
	mode: ApplicableDaysMode;
	days?: (string | number)[];
};

export type PeakHours = {
	id: number;
	name: string;
	hours: string[];
	timezone: string;
	applicable_days: ApplicableDays;
	exclude_days: string[];
	country: string;
	created_at: string;
	updated_at: string;
};

type Envelope<T> = { status: boolean; results?: T; record?: T; total?: number; active?: boolean; error_msg?: string };

async function request<T>(url: string, init?: RequestInit): Promise<T> {
	const response = await fetch(url, { credentials: 'same-origin', ...init });
	const body = await response.text();
	let parsed: Envelope<T> | null = null;
	try {
		parsed = body ? (JSON.parse(body) as Envelope<T>) : null;
	} catch {
		/* normalized below */
	}
	if (!response.ok) throw new Error(parsed?.error_msg || `Request failed (${response.status})`);
	return parsed as T;
}

export async function listPeakHours(): Promise<PeakHours[]> {
	const result = await request<Envelope<PeakHours[]>>('/api/v1/peak-hours');
	return result.results ?? [];
}

export type PeakHoursInput = Omit<PeakHours, 'id' | 'created_at' | 'updated_at'>;

export function createPeakHours(input: PeakHoursInput) {
	return request<Envelope<PeakHours>>('/api/v1/peak-hours', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
	});
}

export function updatePeakHours(name: string, input: Omit<PeakHoursInput, 'name'>) {
	return request<Envelope<PeakHours>>(`/api/v1/peak-hours/${encodeURIComponent(name)}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
	});
}

export function deletePeakHours(name: string) {
	return request<Envelope<never>>(`/api/v1/peak-hours/${encodeURIComponent(name)}`, { method: 'DELETE' });
}

export async function isPeakHoursActive(name: string, at?: string): Promise<boolean> {
	const query = at ? `?at=${encodeURIComponent(at)}` : '';
	const result = await request<Envelope<never>>(`/api/v1/peak-hours/${encodeURIComponent(name)}/is-active${query}`);
	return result.active ?? false;
}

const weekdayNames = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'];
const hourRangeRe = /^([01]\d|2[0-3]):([0-5]\d)-([01]\d|2[0-3]):([0-5]\d)$/;
const isoDateRe = /^\d{4}-\d{2}-\d{2}$/;

export function validatePeakHoursDraft(draft: PeakHoursInput): string | null {
	if (!draft.name.trim()) return 'Name is required.';
	if (!draft.hours.length) return 'At least one hours range is required.';
	for (const h of draft.hours) {
		const m = hourRangeRe.exec(h);
		if (!m) return `"${h}" must be an HH:MM-HH:MM range.`;
		const start = Number(m[1]) * 60 + Number(m[2]);
		const end = Number(m[3]) * 60 + Number(m[4]);
		if (start >= end) return `"${h}" must have a start before its end.`;
	}
	if (!draft.timezone.trim()) return 'Timezone is required.';
	const mode = draft.applicable_days.mode;
	if (mode === 'weekdays') {
		const days = draft.applicable_days.days ?? [];
		if (!days.length) return 'Select at least one weekday.';
		if (!days.every((d) => typeof d === 'string' && weekdayNames.includes(d.toLowerCase())))
			return 'Weekdays must be mon..sun.';
	} else if (mode === 'days_of_month') {
		const days = draft.applicable_days.days ?? [];
		if (!days.length) return 'Select at least one day of month.';
		if (!days.every((d) => typeof d === 'number' && Number.isInteger(d) && d >= 1 && d <= 31))
			return 'Days of month must be whole numbers 1-31.';
	} else if (mode !== 'workdays') {
		return 'applicable_days.mode must be workdays, weekdays, or days_of_month.';
	}
	for (const e of draft.exclude_days) {
		if (e === 'weekends' || e === 'holidays') continue;
		const [start, end] = e.split('..');
		if (end !== undefined) {
			if (!isoDateRe.test(start) || !isoDateRe.test(end)) return `"${e}" must be an ISO date range.`;
		} else if (!isoDateRe.test(e)) {
			return `"${e}" must be "weekends", "holidays", an ISO date, or an ISO date range.`;
		}
	}
	return null;
}

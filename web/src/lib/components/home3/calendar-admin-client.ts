export type HolidayInfo = {
	id: number;
	country: string;
	name: string;
	display_seqno: number;
	description: string;
	note: string;
	created_at: string;
	updated_at: string;
};

export type CalendarDate = { holiday_date: string; holiday_info_id: number; holiday_info_name: string };

export type Calendar = { id: number; year: number; country: string; calendar_type: string; dates: CalendarDate[] };

type Envelope<T> = { status: boolean; results?: T; record?: T; total?: number; error_msg?: string };

async function request<T>(url: string, init?: RequestInit): Promise<Envelope<T>> {
	const response = await fetch(url, { credentials: 'same-origin', ...init });
	const body = await response.text();
	let parsed: Envelope<T> | null = null;
	try {
		parsed = body ? (JSON.parse(body) as Envelope<T>) : null;
	} catch {
		/* normalized below */
	}
	if (!response.ok) throw new Error(parsed?.error_msg || `Request failed (${response.status})`);
	return parsed ?? { status: true };
}

const jsonHeaders = { 'Content-Type': 'application/json' };

export async function listHolidayInfo(country = ''): Promise<HolidayInfo[]> {
	const result = await request<HolidayInfo[]>(`/api/v1/calendars/holiday-info${country ? `?country=${encodeURIComponent(country)}` : ''}`);
	return result.results ?? [];
}

export type HolidayInfoInput = { country: string; name: string; description: string; note: string; display_seqno?: number };

export async function createHolidayInfo(input: HolidayInfoInput): Promise<HolidayInfo> {
	const result = await request<HolidayInfo>('/api/v1/calendars/holiday-info', { method: 'POST', headers: jsonHeaders, body: JSON.stringify(input) });
	return result.record as HolidayInfo;
}

export async function updateHolidayInfo(id: number, input: HolidayInfoInput): Promise<HolidayInfo> {
	const result = await request<HolidayInfo>(`/api/v1/calendars/holiday-info/${id}`, { method: 'PUT', headers: jsonHeaders, body: JSON.stringify(input) });
	return result.record as HolidayInfo;
}

export async function deleteHolidayInfo(id: number): Promise<void> {
	await request<void>(`/api/v1/calendars/holiday-info/${id}`, { method: 'DELETE' });
}

export async function getCalendar(year: number, country: string, calendarType: string): Promise<Calendar> {
	const params = new URLSearchParams({ year: String(year), country, calendar_type: calendarType });
	const result = await request<Calendar>(`/api/v1/calendars?${params.toString()}`);
	return result.record as Calendar;
}

export async function upsertCalendarDates(
	year: number,
	country: string,
	calendarType: string,
	dates: string[],
	holidayInfoId: number
): Promise<Calendar> {
	const result = await request<Calendar>('/api/v1/calendars/dates', {
		method: 'PUT',
		headers: jsonHeaders,
		body: JSON.stringify({ year, country, calendar_type: calendarType, dates, holiday_info_id: holidayInfoId })
	});
	return result.record as Calendar;
}

export async function deleteCalendarDate(calendarId: number, date: string): Promise<void> {
	await request<void>(`/api/v1/calendars/${calendarId}/dates/${date}`, { method: 'DELETE' });
}

export async function deleteCalendar(calendarId: number): Promise<void> {
	await request<void>(`/api/v1/calendars/${calendarId}`, { method: 'DELETE' });
}

export async function getDefaultCountry(): Promise<string | null> {
	const result = await request<{ country: string }>('/api/v1/calendars/default-country');
	return result.record?.country ?? null;
}

export async function setDefaultCountry(country: string): Promise<void> {
	await request<void>('/api/v1/calendars/default-country', { method: 'PUT', headers: jsonHeaders, body: JSON.stringify({ country }) });
}

export async function clearDefaultCountry(): Promise<void> {
	await request<void>('/api/v1/calendars/default-country', { method: 'DELETE' });
}

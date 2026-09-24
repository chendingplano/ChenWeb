import { describe, expect, test } from 'bun:test';
import { validatePeakHoursDraft, type PeakHoursInput } from './peak-hours-client';

const deepSeek: PeakHoursInput = {
	name: 'deepseek',
	hours: ['09:00-12:00', '14:00-18:00'],
	timezone: 'Asia/Shanghai',
	applicable_days: { mode: 'workdays' },
	exclude_days: ['holidays', 'weekends'],
	country: 'CN'
};

describe('peak hours draft validation', () => {
	test('accepts the DeepSeek worked example', () => {
		expect(validatePeakHoursDraft(deepSeek)).toBeNull();
	});
	test('requires a name', () => {
		expect(validatePeakHoursDraft({ ...deepSeek, name: '  ' })).toContain('Name');
	});
	test('requires at least one hours range', () => {
		expect(validatePeakHoursDraft({ ...deepSeek, hours: [] })).toContain('hours range');
	});
	test('rejects malformed hour ranges', () => {
		expect(validatePeakHoursDraft({ ...deepSeek, hours: ['9:00-12:00'] })).toContain('HH:MM-HH:MM');
		expect(validatePeakHoursDraft({ ...deepSeek, hours: ['12:00-09:00'] })).toContain('start before');
	});
	test('requires a timezone', () => {
		expect(validatePeakHoursDraft({ ...deepSeek, timezone: ' ' })).toContain('Timezone');
	});
	test('validates weekdays mode', () => {
		expect(
			validatePeakHoursDraft({ ...deepSeek, applicable_days: { mode: 'weekdays', days: [] } })
		).toContain('weekday');
		expect(
			validatePeakHoursDraft({ ...deepSeek, applicable_days: { mode: 'weekdays', days: ['funday'] } })
		).toContain('mon..sun');
		expect(
			validatePeakHoursDraft({ ...deepSeek, applicable_days: { mode: 'weekdays', days: ['mon', 'fri'] } })
		).toBeNull();
	});
	test('validates days_of_month mode', () => {
		expect(
			validatePeakHoursDraft({ ...deepSeek, applicable_days: { mode: 'days_of_month', days: [0] } })
		).toContain('1-31');
		expect(
			validatePeakHoursDraft({ ...deepSeek, applicable_days: { mode: 'days_of_month', days: [1, 15] } })
		).toBeNull();
	});
	test('rejects invalid exclude_days entries', () => {
		expect(validatePeakHoursDraft({ ...deepSeek, exclude_days: ['not-a-date'] })).toContain('ISO date');
		expect(validatePeakHoursDraft({ ...deepSeek, exclude_days: ['2026-12-24..2026-12-31'] })).toBeNull();
	});
});

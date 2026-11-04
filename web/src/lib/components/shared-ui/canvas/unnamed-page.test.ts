import { test, expect } from 'bun:test';
import { unnamedPageID } from './unnamed-page';

test('uses local date and time for an unnamed page ID', () => {
	expect(unnamedPageID(new Date(2026, 9, 30, 9, 4, 5), [])).toBe('unnamed-20261030-090405');
});

test('chooses a distinct second when another tab has the same ID', () => {
	expect(unnamedPageID(new Date(2026, 9, 30, 23, 59, 59), ['unnamed-20261030-235959']))
		.toBe('unnamed-20261031-000000');
});

test('labels new tabs with the lowest free number', async () => {
	const { unnamedTabLabel } = await import('./unnamed-page');
	expect(unnamedTabLabel([])).toBe('unnamed-1');
	expect(unnamedTabLabel(['unnamed-1', 'unnamed-3', 'home'])).toBe('unnamed-2');
});

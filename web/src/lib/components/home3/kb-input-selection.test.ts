import { describe, expect, test } from 'bun:test';
import { selectedPageIds, togglePageRecord, togglePageSelection } from './kb-input-selection';

describe('KB input page selection', () => {
	test('selects and clears only the records on the visible page', () => {
		const visibleIds = [11, 12, 13];

		expect(selectedPageIds(new Set([12, 99]), visibleIds, true)).toEqual(new Set([11, 12, 13, 99]));
		expect(selectedPageIds(new Set([11, 12, 13, 99]), visibleIds, false)).toEqual(new Set([99]));
	});

	test('toggles one visible record without changing other selections', () => {
		expect(togglePageRecord(new Set([12, 99]), 11)).toEqual(new Set([11, 12, 99]));
		expect(togglePageRecord(new Set([11, 12, 99]), 12)).toEqual(new Set([11, 99]));
	});

	test('reports whether every visible record is selected', () => {
		expect(togglePageSelection(new Set([11, 12]), [11, 12])).toBe(false);
		expect(togglePageSelection(new Set([11]), [11, 12])).toBe(true);
	});
});

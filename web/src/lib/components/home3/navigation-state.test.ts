import { describe, expect, test } from 'bun:test';
import { selectionUrl, resolveSelection } from './navigation-state';

describe('navigation survives a language reload', () => {
	test('preserves the route, unrelated query parameters and hash', () => {
		const url = selectionUrl(new URL('https://example.com/development?dark=1#details'), {
			itemId: 'sysadmin', childId: 'sysadmin-llm-metrics-gold'
		});
		expect(url.pathname).toBe('/development');
		expect(url.searchParams.get('dark')).toBe('1');
		expect(url.hash).toBe('#details');
		expect(url.searchParams.get('view')).toBe('sysadmin');
		expect(url.searchParams.get('child')).toBe('sysadmin-llm-metrics-gold');
	});

	test('restores nested selections using labels in the new language', () => {
		const nodes = [{ id: 'sysadmin', label: '系统管理', children: [
			{ id: 'llm', label: '模型', children: [{ id: 'gold', label: '金标准' }] }
		] }];
		const url = selectionUrl(new URL('https://example.com/development'), {
			itemId: 'sysadmin', childId: 'gold'
		});
		expect(resolveSelection(url, nodes)).toEqual({
			itemId: 'sysadmin', itemTitle: '系统管理', childId: 'gold', childTitle: '金标准'
		});
	});

	test('clears the previous child when selecting a top-level page', () => {
		const url = selectionUrl(new URL('https://example.com/home3?view=sysadmin&child=gold'), {
			itemId: 'dashboard'
		});
		expect(url.searchParams.has('child')).toBe(false);
	});

	test('ignores unknown, hidden and external navigation targets', () => {
		const url = new URL('https://example.com/home3?view=sysadmin&child=gold');
		expect(resolveSelection(url, [])).toBeNull();
		expect(resolveSelection(url, [{ id: 'sysadmin', label: 'Admin' }])).toBeNull();
		expect(resolveSelection(url, [{ id: 'sysadmin', label: 'Admin', children: [
			{ id: 'gold', label: 'Gold', href: '/elsewhere' }
		] }])).toBeNull();
	});
});

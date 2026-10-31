import { test, expect } from 'bun:test';
import { needsRecovery } from './recovery-content';

test('recovery is written only when the page differs from the last successful recovery', () => {
	const first = { descriptor_id: 'page', elements: [{ value: 'one' }] };
	const second = { descriptor_id: 'page', elements: [{ value: 'two' }] };
	expect(needsRecovery(first, undefined)).toBe(true);
	const saved = JSON.stringify(first);
	expect(needsRecovery(first, saved)).toBe(false);
	expect(needsRecovery(second, saved)).toBe(true);
	// A failed write must not advance the successful baseline.
	expect(needsRecovery(second, saved)).toBe(true);
	expect(needsRecovery(second, JSON.stringify(second))).toBe(false);
});

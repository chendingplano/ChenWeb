import test from 'node:test';
import assert from 'node:assert/strict';
import { normalizeFields } from './name-value';

test('normalizeFields wraps strings and drops blank values and empty fields', () => {
	const out = normalizeFields([
		{ key: 'a', label: 'A', values: ['x', '  ', null, { text: 'y', mono: true }] },
		{ key: 'b', label: 'B', values: [undefined, ''] }
	]);
	assert.deepEqual(out, [
		{ key: 'a', label: 'A', values: [{ text: 'x' }, { text: 'y', mono: true }] }
	]);
});

test('normalizeFields keeps fields without values when keepEmpty is set', () => {
	const out = normalizeFields(
		[
			{ key: 'a', label: 'A', values: ['x'] },
			{ key: 'b', label: 'B', values: [null, ' '] }
		],
		true
	);
	assert.deepEqual(out, [
		{ key: 'a', label: 'A', values: [{ text: 'x' }] },
		{ key: 'b', label: 'B', values: [] }
	]);
});

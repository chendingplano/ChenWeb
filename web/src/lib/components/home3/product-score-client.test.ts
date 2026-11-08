import test from 'node:test';
import assert from 'node:assert/strict';
import { evidenceTargets } from './product-score-client';

test('evidence spans split into plain lines and table rows', () => {
	assert.deepEqual(evidenceTargets(['128', '131#h0', '131#r1', '150-152', '134#r5']), {
		lines: [128, 150, 151, 152],
		tables: [
			{ line: 131, rows: ['h0', 'r1'] },
			{ line: 134, rows: ['r5'] }
		]
	});
});

test('unknown span forms and non-arrays are ignored', () => {
	assert.deepEqual(evidenceTargets(['x', '12:14', null]), { lines: [], tables: [] });
	assert.deepEqual(evidenceTargets(undefined), { lines: [], tables: [] });
});

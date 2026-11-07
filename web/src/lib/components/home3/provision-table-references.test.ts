import test from 'node:test';
import assert from 'node:assert/strict';
import { DOMParser } from 'linkedom';
import { provisionTableReferences } from './provision-table-references';

Object.assign(globalThis, { DOMParser });
const refs = [{ line: 121, rows: ['r2'] }];
const lines = [{ line_number: 121, content: '<table><tr><td>A</td><td>B</td><td>C</td><td>D</td></tr><tr><td rowspan="2">1</td><td rowspan="2">Waste</td><td>Machine</td><td>Machine requirement.</td></tr><tr><td>Solar</td><td>Solar requirement. More text.</td></tr></table>' }];

test('legacy cell provisions resolve the unique text match with rowspan-aware columns', () => {
	assert.deepEqual(provisionTableReferences('table_cell', 'Solar requirement.', refs, lines),
		[{ line: 121, rows: ['r2'], cells: ['r2:c4'] }]);
});

test('row provisions, ambiguous matches and missing text keep the original references', () => {
	for (const [kind, text] of [['table_row', 'Solar requirement.'], ['table_cell', 'requirement.'], ['table_cell', 'Missing']]) {
		assert.deepEqual(provisionTableReferences(kind, text, refs, lines), refs);
	}
});

test('explicit cell references and row hashes are preserved', () => {
	const explicit = [{ ...refs[0], cells: ['r2:c3'], row_hash: { r2: 'saved' } }];
	assert.deepEqual(provisionTableReferences('table_cell', 'Solar requirement.', explicit, lines), explicit);
});

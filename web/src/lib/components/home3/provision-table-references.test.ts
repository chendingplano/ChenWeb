import test from 'node:test';
import assert from 'node:assert/strict';
import { DOMParser } from 'linkedom';
import { readFileSync } from 'node:fs';
import { resolveTableReferences, type TableGeometry } from './pdf-table-geometry';
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

test('a line-only provision finds its unique cell on a continuation row', () => {
	assert.deepEqual(provisionTableReferences('table_cell', 'Solar requirement.', [], lines),
		[{ line: 121, cells: ['r2:c4'] }]);
});

test('record 416 resolves the solar provision to its page 7 cell', () => {
	const content = readFileSync('../server/api/documentgeometry/testdata/record416-line.txt', 'utf8').split('\t').slice(6).join('\t');
	const references = provisionTableReferences('table_cell', '利用太阳能辅助堆肥方式处理易腐垃圾,应符合CJJ 52的要求。', [], [{ line_number: 121, content }]);
	const geometry: TableGeometry = { version: 1, algorithm: 'canonical-table-geometry-v5', coordinate_space: 'page-normalized-1000', tables: [{ line: 121, rows: [{ id: 'r2', hash: '', boxes: [], cells: [{ id: 'r2:c4', boxes: [{ page: 7, coords: [440, 100, 750, 200] }] }] }] }] };
	assert.deepEqual(resolveTableReferences(references, geometry), [{ line: 121, target: 'r2:c4', page: 7, coords: [440, 100, 750, 200] }]);
});

test('line-only inference rejects ambiguous matches', () => {
	assert.deepEqual(provisionTableReferences('table_cell', 'requirement.', [], lines), []);
});

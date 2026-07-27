import test from 'node:test';
import assert from 'node:assert/strict';

import {
	addColumn,
	removeColumn,
	renameColumnTitle,
	setColumnAlign,
	addRow,
	removeRow
} from './table-ops.js';
import type { Block } from './types.js';

function table(): Block {
	return {
		id: 'table1',
		type: 'table',
		columns: [
			{ key: 'a', title: 'A' },
			{ key: 'b', title: 'B' }
		],
		rows: [{ cells: { a: [{ type: 'text', text: '1' }], b: [{ type: 'text', text: '2' }] } }]
	};
}

test('addColumn appends a new column with an unused key and backfills every row', () => {
	const result = addColumn(table(), 'C');
	assert.deepStrictEqual(
		result.columns!.map((c) => c.key),
		['a', 'b', 'c']
	);
	assert.equal(result.columns![2].title, 'C');
	assert.deepStrictEqual(result.rows![0].cells['c'], [{ type: 'text', text: '' }]);
});

test('addColumn does not mutate the original block', () => {
	const original = table();
	addColumn(original, 'C');
	assert.equal(original.columns!.length, 2, 'original block must be unchanged');
});

test('addColumn skips already-used single-letter keys', () => {
	const t = table();
	t.columns!.push({ key: 'c', title: 'Existing C' });
	const result = addColumn(t, 'D');
	assert.equal(result.columns![result.columns!.length - 1].key, 'd');
});

test('removeColumn drops the column and its cells from every row', () => {
	const result = removeColumn(table(), 'a');
	assert.deepStrictEqual(
		result.columns!.map((c) => c.key),
		['b']
	);
	assert.equal('a' in result.rows![0].cells, false);
	assert.deepStrictEqual(result.rows![0].cells['b'], [{ type: 'text', text: '2' }]);
});

test('renameColumnTitle changes only the named column', () => {
	const result = renameColumnTitle(table(), 'a', 'String 1');
	assert.equal(result.columns![0].title, 'String 1');
	assert.equal(result.columns![1].title, 'B');
});

test('setColumnAlign sets an align value', () => {
	const result = setColumnAlign(table(), 'a', 'right');
	assert.equal(result.columns![0].align, 'right');
});

test('setColumnAlign with an empty string clears align by omitting the key, not setting it empty', () => {
	const withAlign = setColumnAlign(table(), 'a', 'right');
	const cleared = setColumnAlign(withAlign, 'a', '');
	assert.equal(
		'align' in cleared.columns![0],
		false,
		'align key should be absent, not an empty string'
	);
});

test('addRow appends a row with an empty cell for every declared column', () => {
	const result = addRow(table());
	assert.equal(result.rows!.length, 2);
	assert.deepStrictEqual(result.rows![1].cells, {
		a: [{ type: 'text', text: '' }],
		b: [{ type: 'text', text: '' }]
	});
});

test('removeRow removes exactly the row at the given index', () => {
	const t = addRow(table());
	const result = removeRow(t, 0);
	assert.equal(result.rows!.length, 1);
	assert.deepStrictEqual(result.rows![0].cells.a, [{ type: 'text', text: '' }]);
});

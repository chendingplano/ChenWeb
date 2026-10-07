import test from 'node:test';
import assert from 'node:assert/strict';
import { resolveTableReferences, type TableGeometry } from './pdf-table-geometry';

const geometry: TableGeometry = {
	version: 1,
	algorithm: 'canonical-table-geometry-v2',
	coordinate_space: 'page-normalized-1000',
	tables: [
		{
			line: 121,
			rows: [
				{
					id: 'r3',
					hash: '5364693c56e5',
					boxes: [{ page: 7, coords: [105, 270, 917, 398] }],
					cells: [
						{ id: 'r3:c1', boxes: [{ page: 7, coords: [105, 170, 180, 398] }] },
						{ id: 'r3:c4', boxes: [{ page: 7, coords: [440, 270, 750, 398] }] }
					]
				}
			]
		}
	]
};

test('a row reference resolves directly to continuation page geometry', () => {
	const boxes = resolveTableReferences(
		[{ line: 121, rows: ['r3'], row_hash: { r3: '5364693c56e5' } }],
		geometry
	);
	assert.deepEqual(boxes, [{ page: 7, coords: [105, 270, 917, 398], line: 121, target: 'r3' }]);
});

test('cell selection resolves only the requested physical cell, including spans', () => {
	const boxes = resolveTableReferences([{ line: 121, cells: ['r3:c1'] }], geometry);
	assert.deepEqual(boxes, [{ page: 7, coords: [105, 170, 180, 398], line: 121, target: 'r3:c1' }]);
});

test('stale hashes reject both row and cell references', () => {
	assert.deepEqual(
		resolveTableReferences(
			[{ line: 121, rows: ['r3'], cells: ['r3:c4'], row_hash: { r3: 'stale' } }],
			geometry
		),
		[]
	);
});

test('rows take precedence over their cells, duplicates and missing targets are ignored', () => {
	assert.equal(
		resolveTableReferences(
			[
				{ line: 121, rows: ['r3'], cells: ['r3:c4'] },
				{ line: 121, rows: ['r3', 'r9'] }
			],
			geometry
		).length,
		1
	);
	assert.deepEqual(resolveTableReferences([{ line: 121 }], geometry), []);
	assert.deepEqual(resolveTableReferences([], geometry), []);
	assert.deepEqual(resolveTableReferences([{ line: 121, rows: ['r3'] }], null), []);
});

test('unsupported schemas and invalid coordinates never produce highlights', () => {
	assert.deepEqual(
		resolveTableReferences([{ line: 121, rows: ['r3'] }], { ...geometry, version: 99 }),
		[]
	);
	const malformed = structuredClone(geometry);
	malformed.tables[0].rows[0].boxes[0].coords = [0, 0, NaN, 20];
	assert.deepEqual(resolveTableReferences([{ line: 121, rows: ['r3'] }], malformed), []);
});

test('intrinsically rotated geometry can be displayed with rotation disabled', () => {
	const rotated = structuredClone(geometry);
	rotated.tables[0].rows[0].boxes = [{ page: 7, rotation: 90, coords: [600, 100, 800, 400] }];
	assert.deepEqual(
		resolveTableReferences([{ line: 121, rows: ['r3'] }], rotated, false)[0].coords,
		[100, 200, 400, 400]
	);
});

test('a stale row selection cannot suppress a valid cell selection', () => {
	const boxes = resolveTableReferences(
		[
			{ line: 121, rows: ['r3'], row_hash: { r3: 'stale' } },
			{ line: 121, cells: ['r3:c4'] }
		],
		geometry
	);
	assert.equal(boxes.length, 1);
	assert.equal(boxes[0].target, 'r3:c4');
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { metricTableHighlights, type PdfTextBox } from './metric-pdf-table-highlights';

const pages = new Map<number, PdfTextBox[]>([
	[6, [{ text: '机器成肥', coords: [300, 740, 400, 800] }]],
	[
		7,
		[
			{ text: '太阳能辅助堆肥', coords: [300, 160, 400, 220] },
			{ text: '厌氧产沼发酵', coords: [300, 260, 400, 280] },
			{ text: '容积在50立方米以下的农村户用', coords: [420, 240, 700, 260] },
			{ text: '沼气池应符合NY/T 90的要求。', coords: [420, 260, 700, 280] },
			{ text: '卫生填埋', coords: [300, 320, 400, 350] }
		]
	]
]);
const metric = {
	source_table_rows: [{ line: 121, rows: ['r3'], row_hash: { r3: '5364693c56e5' } }],
	table_context: [
		{
			line: 121,
			columns: [],
			rows: [
				{ id: 'r2', cells: ['1', '易腐垃圾', '太阳能辅助堆肥'] },
				{
					id: 'r3',
					cells: [
						'1',
						'易腐垃圾',
						'厌氧产沼发酵',
						'容积在50立方米以下的农村户用沼气池应符合NY/T 90的要求。'
					],
					matched: true
				},
				{ id: 'r4', cells: ['2', '其他垃圾', '卫生填埋'] }
			]
		}
	]
};

test('121#r3 resolves to its row on page 7, not the first table fragment on page 6', () => {
	assert.deepEqual(metricTableHighlights(metric, pages), [
		{ page: 7, line: 121, row: 'r3', coords: [300, 240, 700, 280] }
	]);
});

test('row refs are authoritative even when popup matched flags differ', () => {
	const other = { ...metric, source_table_rows: [{ line: 121, rows: ['r2'] }] };
	assert.deepEqual(
		metricTableHighlights(other, pages).map((r) => r.row),
		['r2']
	);
});

test('missing or ambiguous PDF text produces no guessed row rectangle', () => {
	assert.deepEqual(metricTableHighlights(metric, new Map()), []);
	const repeated = new Map(pages);
	repeated.set(8, pages.get(7)!);
	assert.deepEqual(metricTableHighlights(metric, repeated), []);
});

test('normalizes PDF whitespace and punctuation while preserving coordinates', () => {
	const split = new Map([[7, [{ text: '厌 氧产沼 发酵', coords: [300, 260, 400, 280] }]]]);
	assert.deepEqual(metricTableHighlights(metric, split)[0]?.coords, [300, 260, 400, 280]);
});

test('a repeated row label is included only beside its uniquely matched row evidence', () => {
	const repeated = new Map(pages);
	repeated.set(6, [{ text: '厌氧产沼发酵', coords: [100, 400, 200, 420] }]);
	assert.deepEqual(metricTableHighlights(metric, repeated), [
		{ page: 7, line: 121, row: 'r3', coords: [300, 240, 700, 280] }
	]);
});

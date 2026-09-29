import test from 'node:test';
import assert from 'node:assert/strict';

import { clipTableCell, splitTableContextRows, type TableContextWindow } from './metric-table-context.js';

const window416: TableContextWindow = {
	line: 116,
	caption: '表1 易腐垃圾及其他垃圾主要处理模式',
	columns: ['序号', '垃圾类型', '处理模式', '技术要求', '适用范围'],
	rows: [
		{ id: 'h0', cells: ['序号', '垃圾类型', '处理模式', '技术要求', '适用范围'], header: true },
		{ id: 'r1', cells: ['1', '易腐垃圾', '机器成肥', '…比能耗…', '人口密度高'], matched: true },
		{ id: 'r2', cells: ['1', '易腐垃圾', '太阳能辅助堆肥', '…', '人口密度不高'] }
	]
};

test('splitTableContextRows separates header and data rows, keeping the matched flag', () => {
	const { head, body } = splitTableContextRows(window416);
	assert.deepEqual(head.map((r) => r.id), ['h0']);
	assert.deepEqual(body.map((r) => r.id), ['r1', 'r2']);
	assert.equal(body[0].matched, true);
	assert.equal(body[1].matched, undefined);
});

test('clipTableCell truncates by characters, not bytes', () => {
	assert.equal(clipTableCell('短文本'), '短文本');
	const long = '长'.repeat(100);
	const clipped = clipTableCell(long, 10);
	assert.equal(Array.from(clipped).length, 10);
	assert.ok(clipped.endsWith('…'));
});

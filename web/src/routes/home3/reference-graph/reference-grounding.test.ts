import { expect, test } from 'bun:test';
import { resolveReferenceLines, paintReferenceHighlights } from './reference-grounding';

const lines = [
	{ line_number: 10, page_number: 2, coords: [100, 600, 800, 650], content: 'Citation', line_type: 'text' },
	{ line_number: 11, page_number: 3, coords: [100, 200, 800, 250], content: 'Target', line_type: 'text' },
	{ line_number: 12, page_number: 3, coords: [100, 300, 800, 350], content: 'Table', line_type: 'table' }
];

test('resolves and orders PDF locations including table lines without spans', () => {
	expect(resolveReferenceLines(['11', '10:11', '999'], [{ line: 12, rows: ['r1'] }], lines).map(line => [line.page_number, line.line_number]))
		.toEqual([[2, 10], [3, 11], [3, 12]]);
	expect(resolveReferenceLines([], [], lines)).toEqual([]);
});

test('paints only the requested page and leaves table rows to the shared viewer', () => {
	const marks: any[] = [];
	const overlay = { ownerDocument: { createElement: () => ({ style: {} }) }, appendChild: (mark: any) => marks.push(mark) } as unknown as HTMLDivElement;
	paintReferenceHighlights(lines, [{ line: 12, rows: ['r1'] }], 3, { width: 500, height: 1000 }, overlay);
	expect(marks).toHaveLength(1);
	expect(marks[0].className).toBe('pdf-highlight');
	expect(marks[0].style.left).toBe('50px');
	expect(marks[0].style.top).toBe('200px');
	expect(marks[0].style.width).toBe('350px');
	expect(marks[0].style.height).toBe('50px');
});

test('document citations locate the complete title, while passage spans take priority', () => {
	const title = { ...lines[0], page_number: 1, content: 'Technical Standard — Document title' };
	expect(resolveReferenceLines([], [], [title, ...lines], 'Document title')).toEqual([title]);
	expect(resolveReferenceLines(['11'], [], [title, ...lines], 'Document title')).toEqual([lines[1]]);
	expect(resolveReferenceLines([], [], lines, 'Document title')).toEqual([]);
});


test('prefers an exact document title over earlier prose containing the title', () => {
	const prose = { ...lines[0], page_number: 1, content: 'See Document title for details.' };
	const title = { ...lines[1], page_number: 2, content: 'Document title' };
	expect(resolveReferenceLines([], [], [prose, title], 'Document title')).toEqual([title]);
});

import test from 'node:test';
import assert from 'node:assert/strict';

import {
	DOC_STRUCTURE_FILTER_OPTIONS,
	filterDocStructureLines,
	getDocStructureFilterLabel
} from './doc-structure-filters.js';

const sampleLines = [
	{
		line_number: 1,
		page_number: 1,
		line_type: 'paragraph',
		corrected_line_type: 'unchanged',
		font: 'Test',
		font_size: '12',
		coords: [0, 0, 1, 1],
		content: 'Body copy'
	},
	{
		line_number: 2,
		page_number: 1,
		line_type: 'paragraph',
		corrected_line_type: 'heading-2',
		font: 'Test',
		font_size: '12',
		coords: [0, 0, 1, 1],
		content: 'Heading copy'
	},
	{
		line_number: 3,
		page_number: 1,
		line_type: 'list-item',
		corrected_line_type: 'unchanged',
		font: 'Test',
		font_size: '12',
		coords: [0, 0, 1, 1],
		content: 'Bullet copy'
	},
	{
		line_number: 4,
		page_number: 1,
		line_type: 'table',
		corrected_line_type: 'unchanged',
		font: 'Test',
		font_size: '12',
		coords: [0, 0, 1, 1],
		content: 'Table copy'
	},
	{
		line_number: 5,
		page_number: 1,
		line_type: 'paragraph',
		corrected_line_type: 'formula',
		font: 'Test',
		font_size: '12',
		coords: [0, 0, 1, 1],
		content: 'E = mc^2'
	}
];

test('doc structure filter options expose the pulldown choices in display order', () => {
	assert.deepEqual(
		DOC_STRUCTURE_FILTER_OPTIONS.map((option) => option.value),
		['all', 'headings', 'paragraphs', 'lists', 'tables', 'formulas']
	);
});

test('doc structure line filters use corrected line types when available', () => {
	assert.deepEqual(
		filterDocStructureLines(sampleLines, 'headings').map((line) => line.content),
		['Heading copy']
	);
	assert.deepEqual(
		filterDocStructureLines(sampleLines, 'paragraphs').map((line) => line.content),
		['Body copy']
	);
	assert.deepEqual(
		filterDocStructureLines(sampleLines, 'lists').map((line) => line.content),
		['Bullet copy']
	);
	assert.deepEqual(
		filterDocStructureLines(sampleLines, 'tables').map((line) => line.content),
		['Table copy']
	);
	assert.deepEqual(
		filterDocStructureLines(sampleLines, 'formulas').map((line) => line.content),
		['E = mc^2']
	);
});

test('doc structure filter labels fall back to All Lines for unknown values', () => {
	assert.equal(getDocStructureFilterLabel('lists'), 'Lists');
	assert.equal(getDocStructureFilterLabel('not-a-real-filter'), 'All Lines');
});

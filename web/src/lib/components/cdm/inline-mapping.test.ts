import test from 'node:test';
import assert from 'node:assert/strict';

import { buildInlineSchema } from './inline-schema.js';
import { inlineArrayToProseMirrorDoc, proseMirrorDocToInlineArray } from './inline-mapping.js';
import type { Inline } from './types.js';

const schema = buildInlineSchema();

function roundTrip(inline: Inline[]): Inline[] {
	const doc = inlineArrayToProseMirrorDoc(schema, inline);
	return proseMirrorDocToInlineArray(doc);
}

test('plain text round-trips', () => {
	const inline: Inline[] = [{ type: 'text', text: 'hello world' }];
	assert.deepStrictEqual(roundTrip(inline), inline);
});

test('strong round-trips', () => {
	const inline: Inline[] = [{ type: 'strong', content: [{ type: 'text', text: 'bold' }] }];
	assert.deepStrictEqual(roundTrip(inline), inline);
});

test('emphasis round-trips', () => {
	const inline: Inline[] = [{ type: 'emphasis', content: [{ type: 'text', text: 'italic' }] }];
	assert.deepStrictEqual(roundTrip(inline), inline);
});

test('code round-trips as a leaf, not a wrapper', () => {
	const inline: Inline[] = [{ type: 'code', text: 'x := 1' }];
	assert.deepStrictEqual(roundTrip(inline), inline);
});

test('link with url round-trips', () => {
	const inline: Inline[] = [
		{ type: 'link', url: 'https://example.com', content: [{ type: 'text', text: 'link' }] }
	];
	assert.deepStrictEqual(roundTrip(inline), inline);
});

test('citation round-trips, including an absent locator', () => {
	const withLocator: Inline[] = [
		{ type: 'citation', citation_key: 'winkler1990', locator: 'p. 354' }
	];
	assert.deepStrictEqual(roundTrip(withLocator), withLocator);

	const withoutLocator: Inline[] = [{ type: 'citation', citation_key: 'winkler1990' }];
	assert.deepStrictEqual(roundTrip(withoutLocator), withoutLocator);
});

test('cross_reference round-trips with display content', () => {
	const inline: Inline[] = [
		{
			type: 'cross_reference',
			target: { block_id: 'eq1' },
			content: [{ type: 'text', text: 'cross-reference' }]
		}
	];
	assert.deepStrictEqual(roundTrip(inline), inline);
});

test('cross_reference round-trips with no display content and a cross-document target', () => {
	const inline: Inline[] = [
		{ type: 'cross_reference', target: { document_key: 'doc:other', block_id: 'sec-1' } }
	];
	assert.deepStrictEqual(roundTrip(inline), inline);
});

test('math round-trips carrying the full Equation shape', () => {
	const inline: Inline[] = [
		{
			type: 'math',
			math: {
				display: false,
				parse_status: 'skipped',
				original: { format: 'typst', source: 'a + b = c' }
			}
		}
	];
	assert.deepStrictEqual(roundTrip(inline), inline);
});

// The exact paragraph from cdmfixtures.AllBlockTypes (p1), exercising every
// inline type together in one run -- the strongest single proof that the
// schema and both mapping directions agree with the real fixture data.
test('the AllBlockTypes p1 paragraph round-trips exactly', () => {
	const inline: Inline[] = [
		{ type: 'text', text: 'See ' },
		{ type: 'strong', content: [{ type: 'text', text: 'bold' }] },
		{ type: 'text', text: ' and ' },
		{ type: 'emphasis', content: [{ type: 'text', text: 'italic' }] },
		{ type: 'text', text: ', inline ' },
		{ type: 'code', text: 'x := 1' },
		{ type: 'text', text: ', a ' },
		{ type: 'link', content: [{ type: 'text', text: 'link' }], url: 'https://example.com' },
		{ type: 'text', text: ', a citation ' },
		{ type: 'citation', citation_key: 'winkler1990', locator: 'p. 354' },
		{ type: 'text', text: ', and a ' },
		{
			type: 'cross_reference',
			content: [{ type: 'text', text: 'cross-reference' }],
			target: { block_id: 'eq1' }
		},
		{ type: 'text', text: '.' }
	];
	assert.deepStrictEqual(roundTrip(inline), inline);
});

// Combined marks are not exercised by either shared Go fixture (both only
// ever apply one mark at a time), but CDM's recursive Content model allows
// nesting, so this is the case that actually stresses groupByMarks'
// depth-by-depth grouping rather than a single wrapper layer.
test('combined strong+emphasis on one run round-trips as nested wrappers', () => {
	const inline: Inline[] = [
		{
			type: 'strong',
			content: [{ type: 'emphasis', content: [{ type: 'text', text: 'bold italic' }] }]
		}
	];
	assert.deepStrictEqual(roundTrip(inline), inline);
});

test('adjacent runs sharing the same mark merge into one wrapper, not two', () => {
	const doc = inlineArrayToProseMirrorDoc(schema, [
		{ type: 'strong', content: [{ type: 'text', text: 'bold one' }] },
		{ type: 'strong', content: [{ type: 'text', text: ' bold two' }] }
	]);
	const result = proseMirrorDocToInlineArray(doc);
	// ProseMirror's own Fragment construction already merges adjacent text
	// nodes carrying identical marks into a single text node before
	// groupByMarks ever runs -- confirmed by inspecting doc.toJSON() here,
	// which is exactly the outcome that matters: one strong wrapper, not two.
	assert.equal(result.length, 1, 'two adjacent strong nodes should merge into one');
	assert.deepStrictEqual(result[0], {
		type: 'strong',
		content: [{ type: 'text', text: 'bold one bold two' }]
	});
});

test('a wrapper mark ends where it stops applying, not merged with unrelated text', () => {
	const inline: Inline[] = [
		{ type: 'strong', content: [{ type: 'text', text: 'bold' }] },
		{ type: 'text', text: ' plain ' },
		{ type: 'strong', content: [{ type: 'text', text: 'bold again' }] }
	];
	const result = roundTrip(inline);
	assert.deepStrictEqual(
		result,
		inline,
		'unrelated plain text between two bold runs must not merge them'
	);
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { parseHTML } from 'linkedom';
import { DOMParser } from '@tiptap/pm/model';

import { buildInlineSchema } from './inline-schema.js';

// Task 5.4: pasted font/colour styling must be dropped while the text
// itself is retained. This is not something inline-mapping.ts implements --
// it is a structural guarantee of inline-schema.ts containing no
// presentation mark at all (design D1/D7). ProseMirror's DOMParser walks
// pasted HTML against the schema's own parseHTML rules; an element or style
// declaration with no matching rule is simply not represented, so its text
// content survives (unwrapped) and its styling does not.
//
// Runs against a linkedom-produced DOM rather than a live browser: verified
// empirically that ProseMirror's DOMParser accepts linkedom's DOM node shape
// without special-casing, and it lets this test run in plain Node (bun
// test), matching the rest of this codebase's testing convention, without
// needing the full authenticated browser stack task group 4 already
// documented as unavailable in this environment.

const schema = buildInlineSchema();

function parseHTMLSnippet(html: string) {
	const { document } = parseHTML(`<html><body>${html}</body></html>`);
	return DOMParser.fromSchema(schema).parse(document.body as unknown as Node);
}

test('color, background, and font-family are dropped; text and structure survive', () => {
	const doc = parseHTMLSnippet(
		'<p style="margin:0"><span style="font-family:Arial;color:red;background:yellow">Styled</span> plain text</p>'
	);
	assert.deepStrictEqual(doc.toJSON(), {
		type: 'doc',
		content: [{ type: 'text', text: 'Styled plain text' }]
	});
});

test('a legacy <font> tag with color and size is dropped entirely; its text survives', () => {
	const doc = parseHTMLSnippet('<font color="blue" size="5">legacy font tag</font>');
	assert.deepStrictEqual(doc.toJSON(), {
		type: 'doc',
		content: [{ type: 'text', text: 'legacy font tag' }]
	});
});

test('text-decoration (underline) has no corresponding mark and is dropped', () => {
	const doc = parseHTMLSnippet('<span style="text-decoration:underline">underlined</span>');
	assert.deepStrictEqual(doc.toJSON(), {
		type: 'doc',
		content: [{ type: 'text', text: 'underlined' }]
	});
});

test('<strong>/<b> and font-weight:bold all map to the strong mark, not a style attribute', () => {
	for (const html of [
		'<strong>a</strong>',
		'<b>a</b>',
		'<span style="font-weight:bold">a</span>',
		'<span style="font-weight:700">a</span>'
	]) {
		const doc = parseHTMLSnippet(html);
		assert.deepStrictEqual(doc.toJSON(), {
			type: 'doc',
			content: [{ type: 'text', marks: [{ type: 'strong' }], text: 'a' }]
		});
	}
});

test('<em>/<i> and font-style:italic all map to the emphasis mark', () => {
	for (const html of ['<em>a</em>', '<i>a</i>', '<span style="font-style:italic">a</span>']) {
		const doc = parseHTMLSnippet(html);
		assert.deepStrictEqual(doc.toJSON(), {
			type: 'doc',
			content: [{ type: 'text', marks: [{ type: 'emphasis' }], text: 'a' }]
		});
	}
});

test('a real link keeps its href as the url attribute; everything else on the anchor is dropped', () => {
	const doc = parseHTMLSnippet(
		'<a href="https://example.com" style="color:purple" target="_blank" onclick="evil()">link</a>'
	);
	assert.deepStrictEqual(doc.toJSON(), {
		type: 'doc',
		content: [
			{
				type: 'text',
				marks: [{ type: 'link', attrs: { url: 'https://example.com' } }],
				text: 'link'
			}
		]
	});
});

test('inline code keeps its text and excludes styling, matching the code mark exclusion rule', () => {
	const doc = parseHTMLSnippet('<code style="color:green;background:black">x := 1</code>');
	assert.deepStrictEqual(doc.toJSON(), {
		type: 'doc',
		content: [{ type: 'text', marks: [{ type: 'code' }], text: 'x := 1' }]
	});
});

test('an unrecognized block wrapper (Word/Docs-style paste) does not swallow the text inside it', () => {
	const doc = parseHTMLSnippet(
		'<div class="WordSection1"><p class="MsoNormal" style="mso-margin-top-alt:auto"><span style="font-size:14pt">Pasted from Word</span></p></div>'
	);
	assert.deepStrictEqual(doc.toJSON(), {
		type: 'doc',
		content: [{ type: 'text', text: 'Pasted from Word' }]
	});
});

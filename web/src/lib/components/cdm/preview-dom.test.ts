import test from 'node:test';
import assert from 'node:assert/strict';
import { parseHTML } from 'linkedom';

import { updatePreviewPages } from './preview-dom.js';

test('updatePreviewPages updates an SVG without replacing its mounted root', () => {
	const { document } = parseHTML('<div id="preview"></div>');
	const preview = document.querySelector('#preview') as unknown as HTMLElement | null;
	assert.ok(preview);

	updatePreviewPages(preview, ['<svg viewBox="0 0 100 100"><text x="1">Before</text></svg>']);
	const page = preview.firstElementChild;
	const svg = page?.firstElementChild;
	assert.ok(svg);

	updatePreviewPages(preview, ['<svg viewBox="0 0 200 200"><text x="2">After</text></svg>']);

	assert.equal(preview.firstElementChild, page);
	assert.equal(preview.firstElementChild?.firstElementChild, svg);
	assert.equal(svg.getAttribute('viewBox'), '0 0 200 200');
	assert.equal(svg.textContent, 'After');
});

test('updatePreviewPages adds and removes page wrappers without replacing surviving pages', () => {
	const { document } = parseHTML('<div id="preview"></div>');
	const preview = document.querySelector('#preview') as unknown as HTMLElement | null;
	assert.ok(preview);

	updatePreviewPages(preview, ['<svg><text>Page 1</text></svg>', '<svg><text>Page 2</text></svg>']);
	const firstPage = preview.firstElementChild;

	updatePreviewPages(preview, ['<svg><text>Updated page 1</text></svg>']);

	assert.equal(preview.children.length, 1);
	assert.equal(preview.firstElementChild, firstPage);
	assert.equal(preview.textContent, 'Updated page 1');
});

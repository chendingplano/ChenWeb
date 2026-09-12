import test from 'node:test';
import assert from 'node:assert/strict';

import { formatSessionJson } from './session-json-formatter';

test('renders objects as escaped name-value pairs', () => {
	const html = formatSessionJson({ type: 'text', nested: { enabled: true } });

	assert.match(html, />type<\/span>/);
	assert.match(html, />text<\/span>/);
	assert.match(html, />nested<\/div>/);
	assert.match(html, />enabled<\/span>/);
	assert.match(html, />true<\/span>/);
});

test('renders arrays with indexed name-value pairs', () => {
	const html = formatSessionJson({ items: ['one', { name: 'two' }] });

	assert.match(html, />\[0\]<\/span>/);
	assert.match(html, />one<\/span>/);
	assert.match(html, />\[1\]<\/div>/);
	assert.match(html, />name<\/span>/);
	assert.match(html, />two<\/span>/);
});

test('renders decoded JSON string values with actual line breaks', () => {
	const html = formatSessionJson(JSON.stringify({ text: 'First line\nSecond line' }));

	assert.match(html, /First line\nSecond line/);
	assert.doesNotMatch(html, /\\n/);
});

test('preserves plain strings and escapes HTML', () => {
	const html = formatSessionJson('<script>alert("x")</script>');

	assert.match(html, /&lt;script&gt;alert\(&quot;x&quot;\)&lt;\/script&gt;/);
	assert.doesNotMatch(html, /<script>/);
});

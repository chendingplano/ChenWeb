import test from 'node:test';
import assert from 'node:assert/strict';

import { formatTopicLineSpecs } from './topic-line-specs.ts';

test('formatTopicLineSpecs joins topic line ranges for sidebar display', () => {
	assert.equal(formatTopicLineSpecs(['1-3', '8', '12-14']), '1-3, 8, 12-14');
});

test('formatTopicLineSpecs falls back to an em dash when no usable line ranges exist', () => {
	assert.equal(formatTopicLineSpecs(undefined), '—');
	assert.equal(formatTopicLineSpecs([]), '—');
	assert.equal(formatTopicLineSpecs([' ', '']), '—');
});

import test from 'node:test';
import assert from 'node:assert/strict';

import { getSystemPrompt } from './chad-sessions-client';

test('extracts a system prompt from session metadata', () => {
	assert.equal(getSystemPrompt({ system_prompt: 'You are a coding assistant.' }),
		'You are a coding assistant.'
	);
});

test('returns null when session metadata has no system prompt', () => {
	assert.equal(getSystemPrompt({}), null);
	assert.equal(getSystemPrompt(undefined), null);
});

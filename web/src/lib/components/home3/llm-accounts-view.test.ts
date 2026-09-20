import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

test('deposit form accepts whole-number amounts and explains timestamp behavior', async () => {
	const source = await readFile(new URL('./llm-accounts-view.svelte', import.meta.url), 'utf8');

	assert.match(source, /step=\{manualMode === 'deposit' \? '1' : '0\.01'\} bind:value=\{deposit\.deposit_amount\}/);
	assert.match(source, /<span>Balance After Deposit<\/span><input type="number" step="0\.01"/);
	assert.match(source, /Leave blank to use the current time; use this to backdate a deposit\./);
});

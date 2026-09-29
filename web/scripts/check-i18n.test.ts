import test from 'node:test';
import assert from 'node:assert/strict';

import { checkParity, findHardcodedText, isHardcodedText } from './check-i18n.ts';

test('wordy text is hard-coded; symbols, numbers and single letters are not', () => {
	assert.ok(isHardcodedText('Review'));
	assert.ok(isHardcodedText('审查'));
	for (const t of ['  ', '·', '→', '42', 'L', '(', '&nbsp;', '—'])
		assert.equal(isHardcodedText(t), false, t);
});

test('finds markup text and text attributes, not expressions or code', () => {
	const src = `<script>const x = 'Not scanned';</script>
<h2>Review Metrics</h2>
<p>{m.mrv_intro()}</p>
<input placeholder="Record ID" class="rounded px-2" />
<button aria-label={m.mrv_search()} title="检索">L{n} · {count}</button>
<code>value_range_type</code>
<pre>raw text</pre>
{#if ok}<span>Done</span>{:else}<span>{m.mrv_none()}</span>{/if}
<style>.a { color: red; }</style>`;
	const got = findHardcodedText(src).map((f) => `${f.line}:${f.text}`);
	assert.deepEqual(got, [
		'2:Review Metrics',
		'4:placeholder="Record ID"',
		'5:title="检索"',
		'8:Done'
	]);
});

test('a fully localised component has no findings', () => {
	assert.deepEqual(
		findHardcodedText(`<h2>{m.mrv_title()}</h2><input placeholder={m.mrv_search_placeholder()} />`),
		[]
	);
});

test('parity reports missing and empty messages', () => {
	const errs = checkParity({
		en: { $schema: 'x', a: 'A', b: 'B', c: ' ' },
		'zh-cn': { $schema: 'x', a: '甲', c: '丙', d: '丁' }
	});
	assert.deepEqual(errs, [
		'message "b" is missing in zh-cn.json',
		'message "c" is empty in en.json',
		'message "d" is missing in en.json'
	]);
});

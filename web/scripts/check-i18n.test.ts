import test from 'node:test';
import assert from 'node:assert/strict';

import { checkParity, findHardcodedText, isHardcodedText, looksLikeText } from './check-i18n.ts';

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

test('flags text literals inside markup expressions, not ids, comparisons or call args', () => {
	const src = `<button>{busy ? 'Saving…' : 'Save'}</button>
<span title={open ? 'Hide details' : \`Show \${name}\`}>{status === 'Done' ? m.a() : m.b()}</span>
<p>{fmt(date, 'YYYY-MM-DD')} {x.toLocaleString('en-US')} {v ?? 'n/a'} {lang === 'en' ? 'English' : '中文'}</p>
<div class={on ? 'bg-blue-500 text-white' : 'hidden'}>{r.downloaded ? 'Re-download' : m.c()}</div>
{#each ['Low', 'High'] as p}<i>{p}</i>{/each}`;
	const got = findHardcodedText(src).map((f) => `${f.line}:${f.text}`);
	assert.deepEqual(got, [
		"1:'Saving…'",
		"1:'Save'",
		"2:'Hide details'",
		'2:`Show ${name}`',
		"4:'Re-download'",
		"5:'Low'",
		"5:'High'"
	]);
});

test('looksLikeText separates display text from code strings', () => {
	for (const t of ['Save', 'Re-download', 'No data.', '加载中', 'browse and search topics']) assert.ok(looksLikeText(t), t);
	for (const t of ['metric_id', 'en-US', 'zh-Hans', 'YYYY-MM-DD', 'HH:mm', '/api/v1/x', 'flex items-center gap-2', 'onClick', 'API_KEY', 'English', '中文'])
		assert.equal(looksLikeText(t), false, t);
});

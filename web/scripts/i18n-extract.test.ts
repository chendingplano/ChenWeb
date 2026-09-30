import test from 'node:test';
import assert from 'node:assert/strict';

import {
	convert,
	convertScript,
	decodeEntities,
	keyPrefix,
	messagesAlias,
	scriptCandidates,
	usesLocalM
} from './i18n-extract.ts';

test('key prefix comes from the file name', () => {
	assert.equal(keyPrefix('src/lib/components/home3/db-maint-log-view.svelte'), 'db_maint_log');
});

test('converts text, attributes, sentences with values, plurals and button states', () => {
	const src = `<script lang="ts">
	let n = $state(0);
</script>

<h2>Review Metrics</h2>
<input placeholder="Record ID" />
<p>Reviewing {review.metrics_count} metrics&hellip; {n === 1 ? '' : 's'}</p>
<p>{items.length} item{items.length === 1 ? '' : 's'}</p>
<button>{busy ? 'Saving…' : 'Save'}</button>
<code>value_range_type</code>`;
	const { out, messages } = convert(src, 'mrv', {});
	assert.ok(out.includes(`<script lang="ts">\n\timport { m } from '$lib/paraglide/messages.js';`));
	assert.ok(out.includes('<h2>{m.mrv_review_metrics()}</h2>'));
	assert.ok(out.includes('<input placeholder={m.mrv_record_id()} />'));
	assert.ok(
		out.includes(
			"{m.mrv_item({ itemsCount: items.length, plural: items.length === 1 ? '' : 's' })}"
		)
	);
	assert.ok(out.includes('{busy ? m.mrv_saving() : m.mrv_save()}'));
	assert.ok(out.includes('<code>value_range_type</code>'));
	const byKey = Object.fromEntries(messages.map((x) => [x.key, x.text]));
	assert.equal(byKey.mrv_reviewing_metrics, 'Reviewing {metrics_count} metrics… {plural}');
	assert.equal(byKey.mrv_item, '{itemsCount} item{plural}');
	assert.equal(byKey.mrv_saving, 'Saving…');
});

test('a literal inside a converted sentence becomes a message too', () => {
	const { out, messages } = convert(`<p>Status: {ok ? 'Ready' : 'Failed'}</p>`, 'x', {});
	assert.equal(
		out,
		`<script lang="ts">\n\timport { m } from '$lib/paraglide/messages.js';\n</script>\n\n<p>{m.x_status({ value: ok ? m.x_ready() : m.x_failed() })}</p>`
	);
	assert.deepEqual(messages.map((x) => x.text).sort(), ['Failed', 'Ready', 'Status: {value}']);
});

test('same text reuses one key; an existing different key gets a suffix', () => {
	const { messages } = convert(`<b>Close</b><i>Close</i><u>Open</u>`, 'd', {
		d_open: 'Something else'
	});
	assert.deepEqual(messages, [
		{ key: 'd_close', text: 'Close' },
		{ key: 'd_open_2', text: 'Open' }
	]);
});

test('script strings: candidates skip code strings; approved ones are converted', () => {
	const src = `<script lang="ts">
	let err = $state('');
	const tabs = [{ id: 'jsz', label: 'JetStream' }];
	async function load(id: string) {
		if (status === 'Done') return;
		const r = await fetch('/api/v1/x');
		if (!r.ok) err = \`Failed to load \${id}\`;
		console.log('Loaded fine');
	}
</script>`;
	const c = scriptCandidates('f.svelte', src);
	assert.deepEqual(
		c.map((x) => x.text),
		['JetStream', 'Failed to load {id}']
	);
	const { out } = convertScript(src, 'f', {}, c);
	assert.ok(out.includes("{ id: 'jsz', label: m.f_jetstream() }"));
	assert.ok(out.includes('err = m.f_failed_to_load({ id })'));
	assert.ok(out.includes("status === 'Done'"));
});

test('detects files that use m as a local name', () => {
	assert.ok(usesLocalM('const m = Math.floor(s / 60);'));
	assert.ok(usesLocalM('list.find((m) => m.id === id)'));
	assert.equal(usesLocalM('{m.mrv_title()}'), false);
});

test('decodes HTML entities in markup text', () => {
	assert.equal(decodeEntities('A&nbsp;&amp;&hellip;&#39;'), "A &…'");
});

test('files using m as a local name get the messages as msg', () => {
	const src = `<script lang="ts">
	const m = metrics[0];
</script>
<h2>Metrics</h2><button>{busy ? 'Saving…' : 'Save'}</button>`;
	const { out } = convert(src, 'x', {});
	assert.ok(out.includes("import { m as msg } from '$lib/paraglide/messages.js';"));
	assert.ok(out.includes('<h2>{msg.x_metrics()}</h2>'));
	assert.ok(out.includes('{busy ? msg.x_saving() : msg.x_save()}'));
	assert.equal(
		messagesAlias(`<script>import { m as t } from '$lib/paraglide/messages.js';</script>`),
		't'
	);
});

test('converts dialog text in handlers and text props on components', () => {
	const { out } = convert(
		`<button onclick={() => confirm('Discard your changes?') && reset('k')}>{m.a()}</button>
<Pane itemLabelPlural="Provisions" variant="outline" />`,
		'p',
		{}
	);
	assert.ok(out.includes("onclick={() => confirm(m.p_discard_your_changes()) && reset('k')}"));
	assert.ok(out.includes('<Pane itemLabelPlural={m.p_provisions()} variant="outline" />'));
});

<script lang="ts">
	import PageConfigPanel from '../../src/lib/components/shared-ui/canvas/PageConfigPanel.svelte';
	import { overwriteGetLocale } from '../../src/lib/paraglide/runtime';
	import type { JsonObject } from '../../src/lib/wpi/types';
	let locale = $state<'en' | 'zh-cn'>('en');
	overwriteGetLocale(() => locale);
	let page = $state<JsonObject>({
		descriptor_id: 'demo',
		page_layouter: { layouter: 'single-pane', root: { pane_id: 'main' } },
		panels: {
			main: {
				elem_layouter: {
					layouter: 'flow-layouter',
					class: { config: { column_gap: '2rem', custom: 'retained' } }
				},
				elem_list: []
			}
		}
	});
</script>

<button id="locale" onclick={() => (locale = locale === 'en' ? 'zh-cn' : 'en')}>Locale</button>
<PageConfigPanel
	{page}
	mode="panel"
	width={400}
	selectedPane="main"
	pageLayouters={[]}
	onUpdate={(value) => (page = value)}
	onLayouterChange={() => {}}
	onSelectPane={() => {}}
	onSplit={() => {}}
	onAdd={() => {}}
	onDelete={() => {}}
	onRename={() => {}}
	onMerge={() => {}}
/>
<pre id="page">{JSON.stringify(page)}</pre>

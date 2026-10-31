<script lang="ts">
	import PageConfigPanel from '../../src/lib/components/shared-ui/canvas/PageConfigPanel.svelte';
	import PagePreview from '../../src/lib/components/shared-ui/canvas/PagePreview.svelte';
	import { overwriteGetLocale } from '../../src/lib/paraglide/runtime';
	import type { JsonObject } from '../../src/lib/wpi/types';
	let locale = $state<'en' | 'zh-cn'>('en');
 let mode = $state<'panel' | 'panes'>('panel');
 let darkMode = $state(false);
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
	const nodes = Array.from({ length: 4 }, (_, i) => ({
		id: String(i), type: 'wpi-element', position: { x: i * 10, y: 0 },
		data: { element: 'text', pane: 'main', attrs: { text: `Item ${i}` }, class: { code_module_id: 'text' } }
	}));
	const noop = () => {};
</script>

<button id="locale" onclick={() => (locale = locale === 'en' ? 'zh-cn' : 'en')}>Locale</button>
<button id="mode" onclick={() => (mode = mode === 'panel' ? 'panes' : 'panel')}>Mode</button>
<button id="theme" onclick={() => (darkMode = !darkMode)}>Theme</button>
<PageConfigPanel
	{page}
	{mode}
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
<div style="position:relative; width:1100px; height:400px">
	<PagePreview {darkMode} descriptor={page} {nodes} selectedId={null} selectedPane={null} size={{ width: 1100, height: 400 }} zoom={1}
		onResizeStart={noop} onResize={noop} onSelect={noop} onSelectPane={noop} onMeasure={noop}
		onPaneResize={noop} onOuterResize={noop} onOuterShift={noop} onMeasurePanes={noop} />
</div>
<style>
 :global(.flex) { display:flex; }
 :global(.flex-wrap) { flex-wrap:wrap; }
 :global(.min-w-0) { min-width:0; }
</style>

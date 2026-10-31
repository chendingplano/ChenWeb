<script lang="ts">
	import { resizePageDimension } from '../../src/lib/components/shared-ui/canvas/page-resize';
 import { setNodeActions } from '../../src/lib/components/shared-ui/canvas/node-actions';
 import WpiApp from '../../src/lib/wpi/WpiApp.svelte';
 import type { Bundle } from '../../src/lib/wpi/types';
 import PageConfigPanel from '../../src/lib/components/shared-ui/canvas/PageConfigPanel.svelte';
	import PagePreview from '../../src/lib/components/shared-ui/canvas/PagePreview.svelte';
	import { overwriteGetLocale } from '../../src/lib/paraglide/runtime';
	import type { JsonObject } from '../../src/lib/wpi/types';
	let selectedId = $state<string | null>(null);
 let action = $state('');
 let previewSize = $state({ width: 1100, height: 400 });
 setNodeActions({ duplicate: (id) => action = `duplicate:${id}`, remove: (id) => action = `remove:${id}`, editClass: (id) => action = `edit:${id}` });
 let locale = $state<'en' | 'zh-cn'>('en');
 let mode = $state<'page' | 'panel' | 'panes'>('panel');
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
 const rootFixtures = [
  { id: 'default', root: { pane_id: 'r' } },
  { id: 'percent', root: { pane_id: 'r', width: '80%', height: '50%' } },
  { id: 'absolute', root: { pane_id: 'r', width: '240px', height: '5rem' } }
 ].map(({ id, root }) => ({ id, bundle: { app: { descriptor_id: id, schema_version: '0.5', initial_page: id }, pages: { [id]: { descriptor_id: id, schema_version: '0.5', width: '500px', height: '200px', page_layouter: { layouter: 'single-pane', root }, panels: {} } }, warnings: [] } as unknown as Bundle }));
 const runtimeBundle = { app: { descriptor_id: 'runtime', schema_version: '0.5', initial_page: 'p' }, pages: { p: { descriptor_id: 'p', schema_version: '0.5', page_layouter: { layouter: 'single-pane', root: { pane_id: 'root', background_color: 'red', foreground_color: 'white', split: 'rows', children: [{ pane_id: 'middle', background_color: 'blue', foreground_color: 'yellow', split: 'columns', children: [{ pane_id: 'main' }, { pane_id: 'right' }] }, { pane_id: 'bottom' }] } }, panels: { main: { elem_layouter: { layouter: 'flow-layouter', config: { max_num_elems: 2 } }, background_color: 'purple', elem_list: Array.from({ length: 4 }, (_, i) => ({ element: 'text', text: `Item ${i}`, width: '60px', class: { code_module_id: 'text' } })) } } } }, warnings: [] } as unknown as Bundle;
</script>

<button id="preview-reset" onclick={() => previewSize = { width: 1100, height: 400 }}>Reset preview</button>
<button id="preview-small" onclick={() => previewSize = { width: 500, height: 300 }}>Small preview</button>
<button id="page-mode" onclick={() => mode = mode === 'page' ? 'panel' : 'page'}>Page mode</button>
<button id="nested" onclick={() => {
 page = { ...page, page_layouter: { layouter: 'single-pane', root: { pane_id: 'root', background_color: 'red', foreground_color: 'white', split: 'rows', children: [ { pane_id: 'middle', background_color: 'blue', foreground_color: 'yellow', split: 'columns', children: [{ pane_id: 'main' }, { pane_id: 'second', background_color: 'green', foreground_color: 'black' }] }, { pane_id: 'bottom' } ] } }, panels: { main: { elem_layouter: { layouter: 'flow-layouter' }, background_color: 'purple' } } };
}}>Nested</button>
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
{#each rootFixtures as fixture}
 <div data-fixture={fixture.id} style="width:600px; height:300px"><WpiApp bundle={fixture.bundle} editorPreview /></div>
{/each}
<output id="action">{action}</output>
<div id="runtime"><WpiApp bundle={runtimeBundle} editorPreview /></div>
<pre id="page">{JSON.stringify(page)}</pre>
<div style="position:relative; width:1100px; height:400px">
	<PagePreview {darkMode} descriptor={page} {nodes} {selectedId} selectedPane={null} size={previewSize} zoom={1}
		onResizeStart={noop} onResize={(value) => previewSize = value} onSelect={(id) => selectedId = id} onSelectPane={noop} onMeasure={noop}
		onPaneResize={noop} onPageResize={(axis, value) => page = resizePageDimension(page, axis, value)} onPageCancel={(original) => page = original} onMeasurePanes={noop} />
</div>
<style>
 :global(.flex) { display:flex; }
 :global(.flex-wrap) { flex-wrap:wrap; }
 :global(.min-w-0) { min-width:0; }
</style>

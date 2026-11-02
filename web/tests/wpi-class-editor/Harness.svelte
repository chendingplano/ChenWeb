<script lang="ts">
	import WpiClassPanel from '../../src/lib/components/shared-ui/canvas/WpiClassPanel.svelte';
	import type { ClassTarget } from '../../src/lib/components/shared-ui/canvas/class-editor';
	import { overwriteGetLocale } from '../../src/lib/paraglide/runtime';
	let target = $state<ClassTarget | null>({ kind: 'element', id: 'input' });
	let locale = $state<'en' | 'zh-cn'>('en');
	let saves = $state(0);
	overwriteGetLocale(() => locale);
</script>

<nav>
	<button id="select-input" onclick={() => target = { kind: 'element', id: 'input' }}>Input</button>
	<button id="select-list" onclick={() => target = { kind: 'element', id: 'list' }}>List</button>
	<button id="select-pane" onclick={() => target = { kind: 'page-layouter', id: 'single-pane' }}>Pane</button>
	<button id="select-panel" onclick={() => target = { kind: 'element-layouter', id: 'flow-layouter' }}>Panel</button>
	<button id="select-action" onclick={() => target = { kind: 'action', id: 'save' }}>Action</button>
	<button id="select-page" onclick={() => target = null}>Page</button>
	<button id="language-en" onclick={() => locale = 'en'}>English</button>
	<button id="language-zh" onclick={() => locale = 'zh-cn'}>中文</button>
</nav>
<output id="saves">{saves}</output>
<div class="panel"><WpiClassPanel {target} onSaved={() => saves++}><p id="instance-properties">Instance properties remain separate.</p></WpiClassPanel></div>

<style>
	:global(body) { font-family: sans-serif; margin: 16px; background: #151718; color: white; }
	.panel { width: 390px; height: 800px; border: 1px solid #444; --cv-panel: #1b1d20; --cv-surface: #25282c; --cv-border: #44484d; --cv-divider: #34383c; --cv-text: #fff; --cv-text-2: #e4e7eb; --cv-text-muted: #b8bec7; --cv-text-faint: #9199a4; --cv-accent: #65b9ff; }
	nav { display: flex; gap: 6px; margin-bottom: 12px; }
</style>

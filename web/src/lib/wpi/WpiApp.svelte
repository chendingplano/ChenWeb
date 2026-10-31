<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import PageView from './PageView.svelte';
	import UnsavedDialog from './UnsavedDialog.svelte';
	import { AppRuntime, setRuntime } from './runtime.svelte';
	import type { Bundle } from './types';

	let { bundle, editorPreview = false }: { bundle: Bundle; editorPreview?: boolean } = $props();

	// The route remounts this component for another app, so the runtime is
	// created once from the first bundle. The app shows its initial page in its
	// page area (devdoc 2026102601 §3.1.1).
	const rt = new AppRuntime(untrack(() => bundle), editorPreview);
	setRuntime(rt);
	onMount(() => rt.start());

	function beforeUnload(event: BeforeUnloadEvent) {
		if (rt.anyDirty()) event.preventDefault();
	}
</script>

<svelte:window onbeforeunload={beforeUnload} />

{#if rt.root}
	<div class="h-full w-full">
		<PageView instanceId={rt.root} />
	</div>
{/if}
<UnsavedDialog />

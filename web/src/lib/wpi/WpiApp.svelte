<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import ErrorBox from './ErrorBox.svelte';
	import UnsavedDialog from './UnsavedDialog.svelte';
	import { AppRuntime, setRuntime } from './runtime.svelte';
	import { templates } from './templates';
	import type { Bundle } from './types';

	let { bundle }: { bundle: Bundle } = $props();

	// The route remounts this component for another app, so the runtime is
	// created once from the first bundle.
	const rt = new AppRuntime(untrack(() => bundle));
	setRuntime(rt);
	onMount(() => rt.start());

	const Template = $derived(templates[rt.bundle.app.layout_template]);

	function beforeUnload(event: BeforeUnloadEvent) {
		if (rt.anyDirty()) event.preventDefault();
	}
</script>

<svelte:window onbeforeunload={beforeUnload} />

{#if Template}
	<Template />
{:else}
	<div class="p-4">
		<ErrorBox
			title={m.wpi_error_title({ descriptor: rt.bundle.app.descriptor_id })}
			message={m.wpi_unknown_template({ name: rt.bundle.app.layout_template })}
		/>
	</div>
{/if}
<UnsavedDialog />

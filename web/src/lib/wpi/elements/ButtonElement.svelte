<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { getRuntime } from '../runtime.svelte';
	import { triggerOrigin, type ElementProps } from './index';

	let { panelId, element }: ElementProps = $props();

	const rt = getRuntime();
	const d = $derived(element.desc);
	let busy = $state(false);

	async function click() {
		if (!d.on_click) return;
		busy = true;
		try {
			await rt.runTrigger(d.on_click, triggerOrigin(element, 'on_click'), { panelId });
		} finally {
			busy = false;
		}
	}
</script>

<Button onclick={click} disabled={busy}>{d.label}</Button>

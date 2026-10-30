<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { actionRef } from '../entries';
	import { getRuntime } from '../runtime.svelte';
	import { localized } from '../text';
	import type { ElementProps } from './index';

	let { instanceId, element }: ElementProps = $props();

	const rt = getRuntime();
	const d = $derived(element.desc);
	let busy = $state(false);

	async function click() {
		if (!d.on_click) return;
		busy = true;
		try {
			await rt.runTrigger(d.on_click, actionRef(element, 'on_click'), { instanceId });
		} finally {
			busy = false;
		}
	}
</script>

<Button onclick={click} disabled={busy}>{localized(d.label)}</Button>

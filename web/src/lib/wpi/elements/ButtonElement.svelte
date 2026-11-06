<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { m } from '$lib/paraglide/messages.js';
	import { actionRef } from '../entries';
	import { getRuntime } from '../runtime.svelte';
	import { localized } from '../text';
	import { surfaceColorValue } from '../layout';
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

<Button onclick={click} disabled={busy} class={d.background_color ? 'bg-[var(--button-background)] hover:bg-[color-mix(in_srgb,var(--button-background)_90%,transparent)]' : undefined} style={`${d.width !== undefined && d.width !== null && d.width !== 'auto' ? 'width:100%;' : ''}${d.height !== undefined && d.height !== null && d.height !== 'auto' ? 'height:100%;' : ''}${d.background_color ? `--button-background:${surfaceColorValue(d.background_color)};` : ''}`}>{localized(d.label) || (rt.editorPreview ? m.wpi_editor_button_placeholder() : '')}</Button>

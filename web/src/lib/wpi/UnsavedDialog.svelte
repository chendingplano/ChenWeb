<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { m } from '$lib/paraglide/messages.js';
	import { getRuntime } from './runtime.svelte';

	const rt = getRuntime();
	let dialog: HTMLDialogElement;

	$effect(() => {
		if (rt.prompt && !dialog.open) dialog.showModal();
		if (!rt.prompt && dialog.open) dialog.close();
	});

	// Escape closes the dialog: treat it as "keep editing".
	function cancel(event: Event) {
		event.preventDefault();
		rt.prompt?.resolve('keep');
	}
</script>

<dialog
	bind:this={dialog}
	oncancel={cancel}
	aria-labelledby="wpi-unsaved-title"
	class="bg-background text-foreground m-auto w-[min(26rem,calc(100vw-2rem))] rounded-lg border p-5 shadow-lg backdrop:bg-black/40"
>
	<h2 id="wpi-unsaved-title" class="text-base font-semibold">{m.wpi_unsaved_title()}</h2>
	<p class="text-muted-foreground mt-2 text-sm">{m.wpi_unsaved_body()}</p>
	<div class="mt-5 flex flex-wrap justify-end gap-2">
		<Button variant="outline" onclick={() => rt.prompt?.resolve('keep')}>{m.wpi_unsaved_keep()}</Button>
		<Button variant="outline" onclick={() => rt.prompt?.resolve('discard')}>{m.wpi_unsaved_discard()}</Button>
		{#if rt.prompt?.canSave}
			<Button onclick={() => rt.prompt?.resolve('save')}>{m.wpi_unsaved_save()}</Button>
		{/if}
	</div>
</dialog>

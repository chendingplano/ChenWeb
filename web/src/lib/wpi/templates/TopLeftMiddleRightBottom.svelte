<script lang="ts">
	import Pane from '../Pane.svelte';
	import { getRuntime } from '../runtime.svelte';

	// Regions the app descriptor does not mention are not drawn.
	const panes = getRuntime().bundle.app.panes;
	const has = (id: string) => Object.hasOwn(panes, id);
</script>

<div class="flex h-full w-full flex-col">
	{#if has('top')}<Pane paneId="top" axis="height" class="border-b" />{/if}
	{#if has('left') || has('middle') || has('right')}
		<div class="flex min-h-0 flex-1">
			{#if has('left')}<Pane paneId="left" axis="width" handle="right" class="border-r" />{/if}
			{#if has('middle')}<Pane paneId="middle" axis="width" />{/if}
			{#if has('right')}<Pane paneId="right" axis="width" handle="left" class="border-l" />{/if}
		</div>
	{/if}
	{#if has('bottom')}<Pane paneId="bottom" axis="height" class="border-t" />{/if}
</div>

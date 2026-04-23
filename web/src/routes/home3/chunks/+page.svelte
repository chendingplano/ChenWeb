<script lang="ts">
	import { page } from '$app/state';
	import ChunkMgmtView from '$lib/components/home3/chunk-mgmt-view.svelte';

	let darkMode = $derived(page.url.searchParams.get('dark') !== '0');
</script>

<div class="chunks-standalone">
	<ChunkMgmtView {darkMode} />
</div>

<style>
	.chunks-standalone {
		height: 100vh;
		overflow: hidden;
	}
</style>

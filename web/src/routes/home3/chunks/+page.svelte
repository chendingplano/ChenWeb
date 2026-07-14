<script lang="ts">
	import { theme } from '$lib/stores/theme.svelte';
	import ChunkMgmtView from '$lib/components/home3/chunk-mgmt-view.svelte';

	let darkMode = $derived(theme.isDark);
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

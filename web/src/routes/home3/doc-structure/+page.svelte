<script lang="ts">
	import { page } from '$app/state';
	import DocStructureView from '$lib/components/home3/doc-structure-view.svelte';

	let darkMode = $derived(page.url.searchParams.get('dark') !== '0');
</script>

<div class="doc-structure-standalone">
	<DocStructureView {darkMode} />
</div>

<style>
	.doc-structure-standalone {
		height: 100vh;
		overflow: hidden;
	}
</style>

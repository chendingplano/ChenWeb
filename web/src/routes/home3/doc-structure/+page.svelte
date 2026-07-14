<script lang="ts">
	import { theme } from '$lib/stores/theme.svelte';
	import DocStructureView from '$lib/components/home3/doc-structure-view.svelte';

	let darkMode = $derived(theme.isDark);
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

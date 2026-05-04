<script lang="ts">
	import TreeGraphView from './tree-graph-view.svelte';
	let { darkMode = true }: { darkMode?: boolean } = $props();
</script>

<TreeGraphView mode="summary" {darkMode} />

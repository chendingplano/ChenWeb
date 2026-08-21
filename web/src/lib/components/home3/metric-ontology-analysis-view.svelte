<script lang="ts">
	import MetricOntologyAnalysisPage from '../../../routes/home3/ontology-metric-analysis/+page.svelte';

	let { darkMode, knowledgeStoreId = null }: { darkMode: boolean; knowledgeStoreId?: number | null } = $props();
</script>

<MetricOntologyAnalysisPage embedded hostDarkMode={darkMode} {knowledgeStoreId} />

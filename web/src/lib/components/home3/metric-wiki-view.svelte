<script lang="ts">
	import { getMetricWiki, type MetricWikiPage } from '$lib/services/metricWikiService';
	import ArtifactWikiArticleView from './artifact-wiki-article-view.svelte';

	let {
		metricId,
		darkMode = true,
		lang,
		showInfobox = true
	}: { metricId: string; darkMode?: boolean; lang?: string; showInfobox?: boolean } = $props();

	let page = $state<MetricWikiPage | null>(null);
	let loading = $state(true);
	let generated = $state(false);
	let error = $state('');

	async function load() {
		loading = true;
		error = '';
		try {
			const resp = await getMetricWiki(metricId, lang);
			page = resp.page;
			generated = resp.generated;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	// Load on mount and whenever the metric (or language) changes. The $effect
	// fires once after mount and again on any key change.
	let lastKey = $state('');
	$effect(() => {
		const key = `${metricId}::${lang ?? ''}`;
		if (key !== lastKey) {
			lastKey = key;
			load();
		}
	});
</script>

<ArtifactWikiArticleView
	article={page as unknown as Record<string, unknown>}
	artifactType="metric"
	artifactId={metricId}
	{darkMode}
	{lang}
	{showInfobox}
	{loading}
	{error}
	{generated}
	retry={load}
/>

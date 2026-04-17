<script lang="ts">
	import { page } from '$app/state';
	import MetricMgmtView from '$lib/components/home3/metric-mgmt-view.svelte';

	let darkMode = $derived(page.url.searchParams.get('dark') !== '0');
</script>

<div class="metrics-standalone">
	<MetricMgmtView {darkMode} />
</div>

<style>
	.metrics-standalone {
		height: 100vh;
		overflow: hidden;
	}
</style>

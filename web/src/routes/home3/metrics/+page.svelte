<script lang="ts">
	import { theme } from '$lib/stores/theme.svelte';
	import MetricMgmtView from '$lib/components/home3/metric-mgmt-view.svelte';

	let darkMode = $derived(theme.isDark);
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

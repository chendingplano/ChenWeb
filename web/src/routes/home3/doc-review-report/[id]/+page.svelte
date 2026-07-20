<script lang="ts">
	import { page } from '$app/state';
	import { theme } from '$lib/stores/theme.svelte';
	import DocReviewReportView from '$lib/components/home3/doc-review-report-view.svelte';
	import SiteHeader from '../../../semos/components/SiteHeader.svelte';

	let { data } = $props();

	let dark = $derived(theme.isDark);
	let reportId = $derived(Number(page.params.id));
</script>

<div class="page-shell">
	<SiteHeader config={data.siteConfig} />
	<div class="report-body">
		<DocReviewReportView {reportId} darkMode={dark} />
	</div>
</div>

<style>
	.page-shell {
		display: flex;
		flex-direction: column;
		height: 100vh;
		width: 100%;
	}
	.report-body {
		flex: 1 1 auto;
		min-height: 0;
	}
</style>

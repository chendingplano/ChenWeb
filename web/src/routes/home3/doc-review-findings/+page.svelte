<script lang="ts">
	import { page } from '$app/state';
	import { theme } from '$lib/stores/theme.svelte';
	import DocReviewFindingsView from '$lib/components/home3/doc-review-findings-view.svelte';

	let darkMode = $derived(theme.isDark);
	let initialRunId = $derived(page.url.searchParams.get('run_id')?.trim() ?? '');
</script>

<div class="doc-review-findings-standalone">
	<DocReviewFindingsView {darkMode} {initialRunId} />
</div>

<style>
	.doc-review-findings-standalone {
		height: 100vh;
		overflow: hidden;
	}
</style>

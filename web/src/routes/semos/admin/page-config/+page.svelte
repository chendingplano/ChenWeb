<script lang="ts">
	import { page } from '$app/state';
	import PageConfigAdminView from '$lib/components/home3/page-config-admin-view.svelte';

	// Standalone route for the page-content admin tool. The primary entry point
	// is System Admin → Page Content (rendered in the center panel); this route
	// gives a direct, shareable URL. Theme via ?dark=1/0.
	const darkMode = $derived(page.url.searchParams.get('dark') !== '0');
</script>

<PageConfigAdminView {darkMode} />

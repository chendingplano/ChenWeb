<script lang="ts">
	import { theme } from '$lib/stores/theme.svelte';
	import ProductReviewIntakeView from '$lib/components/home3/product-review-intake-view.svelte';

	// Standalone data app, opened in its own tab with ?dark=; fall back to the
	// site-wide theme store (same contract as /home3/product-metric-review).
	function darkParam(): boolean | null {
		if (typeof window === 'undefined') return null;
		const v = new URLSearchParams(window.location.search).get('dark');
		if (v === '1') return true;
		if (v === '0') return false;
		return null;
	}
	let darkMode = $derived(darkParam() ?? theme.isDark);
</script>

<div class="pmr-standalone">
	<ProductReviewIntakeView {darkMode} />
</div>

<style>
	.pmr-standalone {
		min-height: 100vh;
	}
</style>

<script lang="ts">
	import SiteHeader from '../../semos/components/SiteHeader.svelte';

	let { data, children } = $props();
</script>

<div class="knowledge-shell flex min-h-0 flex-col overflow-hidden">
	<SiteHeader config={data.siteConfig} />
	<main class="min-h-0 flex-1 overflow-hidden">
		{@render children?.()}
	</main>
</div>

<style>
	.knowledge-shell {
		height: 100vh;
		height: 100dvh;
	}

	:global(body:has(.knowledge-shell)) {
		overflow: hidden;
	}
</style>

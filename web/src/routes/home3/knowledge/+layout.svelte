<script lang="ts">
	import SiteHeader from '../../semos/components/SiteHeader.svelte';

	let { data, children } = $props();
</script>

<div class="flex h-dvh min-h-0 flex-col overflow-hidden">
	<SiteHeader config={data.siteConfig} />
	<main class="min-h-0 flex-1 overflow-hidden">
		{@render children?.()}
	</main>
</div>

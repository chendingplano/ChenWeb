<script lang="ts">
	import SiteHeader from '../../semos/components/SiteHeader.svelte';

	let { data, children } = $props();
</script>

<SiteHeader config={data.siteConfig} />
{@render children?.()}

<script lang="ts">
	import Dashboard from '$lib/components/home3/dashboard.svelte';

	let { data } = $props();
</script>

<Dashboard siteConfig={data.siteConfig} />

<script lang="ts">
	import { onMount } from 'svelte';
	import { semosTheme } from '$lib/stores/semosTheme.svelte';
	import SiteHeader from './components/SiteHeader.svelte';
	import SiteFooter from './components/SiteFooter.svelte';

	let { data, children } = $props();

	onMount(() => semosTheme.init());
</script>

<div
	class="flex min-h-screen flex-col bg-[#faf9f7] text-[#17181c] transition-colors duration-300 dark:bg-[#101216] dark:text-[#e9e7e2]"
>
	<SiteHeader config={data.siteConfig} />
	<main class="flex-1">
		{@render children?.()}
	</main>
	<SiteFooter config={data.siteConfig} />
</div>

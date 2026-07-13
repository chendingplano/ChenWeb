<script lang="ts">
	import { onMount } from 'svelte';
	import { semosTheme } from '$lib/stores/semosTheme.svelte';
	import SiteHeader from './components/SiteHeader.svelte';
	import SiteFooter from './components/SiteFooter.svelte';

	let { data, children } = $props();

	onMount(() => semosTheme.init());
</script>

<div class="flex min-h-screen flex-col bg-[#faf9f6] text-[#1a1a1a] transition-colors duration-300 dark:bg-[#0c0f1a] dark:text-[#e8e7e4]">
	<SiteHeader config={data.siteConfig} />
	<main class="flex-1">
		{@render children?.()}
	</main>
	<SiteFooter config={data.siteConfig} />
</div>

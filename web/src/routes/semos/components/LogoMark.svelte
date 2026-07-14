<script lang="ts">
	import type { SiteBranding } from '$lib/services/siteConfigService';

	let {
		branding,
		textClass,
		dotClass = 'h-2.5 w-2.5'
	}: { branding: SiteBranding; textClass: string; dotClass?: string } = $props();
</script>

{#if branding.logo_image}
	<img src={branding.logo_image} alt={branding.logo_text} class="h-7 w-auto" />
{:else}
	<span class="inline-block {dotClass} rotate-45 rounded-[2px] bg-[#b08d57]"></span>
	<span class={textClass}>{branding.logo_text}</span>
{/if}

<script lang="ts">
	import type { SiteConfig } from '$lib/services/siteConfigService';

	let { config }: { config: SiteConfig } = $props();
</script>

<footer class="border-t border-border/70">
	<div
		class="mx-auto flex max-w-6xl flex-col-reverse items-center justify-between gap-3 px-6 py-10 text-sm text-muted-foreground md:flex-row"
	>
		<p>{config.footer.text}</p>
		<p class="text-muted-foreground/80">{config.branding.powered_by}</p>
	</div>
</footer>

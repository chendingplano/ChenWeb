<script lang="ts">
	import { page } from '$app/state';
	import { m } from '$lib/paraglide/messages.js';
	import {
		fetchTenantSiteConfig,
		type SiteConfig
	} from '$lib/services/siteConfigService';
	import {
		Database,
		MessageCircle,
		Search,
		FileCheck,
		Workflow,
		Bot,
		LayoutGrid,
		ArrowUpRight,
		Megaphone,
		Activity,
		AlertTriangle
	} from '@lucide/svelte';

	let { data } = $props();

	let tenantConfig = $state<SiteConfig | null>(null);
	let tenantError = $state<string | null>(null);

	const tenantId = $derived(page.url.searchParams.get('tenant'));
	const cfg = $derived(tenantConfig ?? data.siteConfig);

	$effect(() => {
		tenantConfig = null;
		tenantError = null;
		if (tenantId) {
			fetchTenantSiteConfig(tenantId)
				.then((c) => (tenantConfig = c))
				.catch((e) => (tenantError = String(e)));
		}
	});

	// Icon names in site-config TOML -> lucide components.
	const icons: Record<string, typeof LayoutGrid> = {
		database: Database,
		'message-circle': MessageCircle,
		search: Search,
		'file-check': FileCheck,
		workflow: Workflow,
		bot: Bot
	};

	// Placeholder feed content; real data sources wired in a later phase.
	const announcements = ['Welcome to your SemOS workspace.'];
	const recent: string[] = [];
	const alarms: string[] = [];

	const feeds = $derived([
		{
			title: m.semos_workspace_announcements(),
			icon: Megaphone,
			items: announcements,
			empty: '—',
			accent: 'border-l-primary'
		},
		{
			title: m.semos_workspace_recent(),
			icon: Activity,
			items: recent,
			empty: 'No recent activity.',
			accent: 'border-l-blue-500/50 dark:border-l-blue-400/40'
		},
		{
			title: m.semos_workspace_alarms(),
			icon: AlertTriangle,
			items: alarms,
			empty: 'No alarms.',
			accent: 'border-l-amber-500/50 dark:border-l-amber-400/40'
		}
	]);

	function reveal(node: HTMLElement) {
		const observer = new IntersectionObserver(
			(entries) => {
				for (const entry of entries) {
					if (entry.isIntersecting) {
						entry.target.classList.add('is-visible');
						observer.unobserve(entry.target);
					}
				}
			},
			{ threshold: 0.1, rootMargin: '0px 0px -10% 0px' }
		);
		observer.observe(node);
		return {
			destroy() {
				observer.disconnect();
			}
		};
	}
</script>

<svelte:head>
	<title>{cfg.workspace.banner_title} — {cfg.branding.site_name}</title>
</svelte:head>

<!-- ════════════════════════════════════════════
     Workspace banner — gradient drench, not just
     a tinted overlay
     ════════════════════════════════════════════ -->
<section class="relative overflow-hidden border-b border-border/70">
	<img
		src={cfg.workspace.banner_image}
		alt=""
		class="absolute inset-0 h-full w-full object-cover"
	/>
	<div
		class="absolute inset-0 bg-gradient-to-b from-background/85 via-background/80 to-background/90"
	></div>
	<div class="relative mx-auto max-w-6xl px-6 py-14 md:py-20">
		<h1 class="max-w-[20ch] text-[clamp(1.5rem,2.5vw+0.5rem,2.25rem)] font-semibold leading-[1.15] tracking-tight text-foreground">
			{cfg.workspace.banner_title}
		</h1>
		<p class="mt-3 max-w-[58ch] text-sm leading-relaxed text-muted-foreground md:text-base">
			{cfg.workspace.banner_subtitle}
		</p>
		{#if tenantError}
			<p class="mt-4 flex items-center gap-1.5 text-sm text-destructive">
				<AlertTriangle class="h-4 w-4 shrink-0" />
				{tenantError}
			</p>
		{/if}
	</div>
</section>

<!-- ════════════════════════════════════════════
     Feed cards — each with a coloured left border
     ════════════════════════════════════════════ -->
<div class="mx-auto max-w-6xl px-6 py-10 md:py-14">
	<div
		use:reveal
		class="reveal grid gap-5 sm:grid-cols-2 lg:grid-cols-3"
	>
		{#each feeds as block (block.title)}
			<div
				class="rounded-lg border border-border border-l-[3px] bg-background shadow-sm transition-all duration-200 hover:shadow-md {block.accent}"
			>
				<div class="p-5">
					<div class="flex items-center justify-between">
						<h2
							class="flex items-center gap-1.5 text-xs font-semibold tracking-wider text-muted-foreground uppercase"
						>
							<block.icon class="h-3.5 w-3.5" />
							{block.title}
						</h2>
						{#if block.items.length > 0}
							<span
								class="rounded-full bg-muted px-1.5 py-0.5 text-[11px] font-medium tabular-nums text-muted-foreground"
							>
								{block.items.length}
							</span>
						{/if}
					</div>
					<ul class="mt-3 space-y-2 text-sm leading-relaxed">
						{#if block.items.length === 0}
							<li class="text-muted-foreground/70 italic">{block.empty}</li>
						{:else}
							{#each block.items as item (item)}
								<li class="text-foreground/80">{item}</li>
							{/each}
						{/if}
					</ul>
				</div>
			</div>
		{/each}
	</div>

	<!-- ════════════════════════════════════════════
	     Apps grid — larger tiles with shadow + lift
	     ════════════════════════════════════════════ -->
	<div use:reveal class="reveal mt-14">
		<div class="mb-6 flex items-center gap-3">
			<h2 class="text-lg font-semibold tracking-tight text-foreground">
				{m.semos_workspace_apps()}
			</h2>
			<span class="h-px flex-1 bg-border/50"></span>
		</div>
		<div class="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
			{#each cfg.workspace.apps as app (app.name)}
				{@const Icon = icons[app.icon] ?? LayoutGrid}
				<a
					href={app.href}
					class="group flex flex-col gap-4 rounded-xl border border-border/70 bg-background p-6 shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:border-foreground/15 hover:shadow-md focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
				>
					<div class="flex items-start justify-between">
						<span
							class="inline-flex rounded-xl bg-primary/10 p-3 text-primary ring-1 ring-primary/5"
						>
							<Icon class="h-6 w-6" />
						</span>
						<ArrowUpRight
							class="mt-1 h-4 w-4 text-muted-foreground/40 transition-all duration-200 group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:text-foreground"
						/>
					</div>
					<div>
						<span class="font-semibold tracking-tight text-foreground">{app.name}</span>
						<p class="mt-1 text-sm leading-relaxed text-muted-foreground">
							{app.description}
						</p>
					</div>
				</a>
			{/each}
		</div>
	</div>
</div>

<style>
	.reveal {
		opacity: 0;
		transform: translateY(0.75rem);
		transition:
			opacity 0.45s cubic-bezier(0.16, 1, 0.3, 1),
			transform 0.45s cubic-bezier(0.16, 1, 0.3, 1);
	}
	.reveal:global(.is-visible) {
		opacity: 1;
		transform: translateY(0);
	}
	@media (prefers-reduced-motion: reduce) {
		.reveal {
			opacity: 1;
			transform: none;
			transition: none;
		}
	}
</style>

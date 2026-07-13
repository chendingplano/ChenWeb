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

	// Decorative-only: which lucide icon and item count badge to show per feed
	// column. Not part of the icon-name mapping above.
	const feeds = $derived([
		{
			title: m.semos_workspace_announcements(),
			icon: Megaphone,
			items: announcements,
			empty: '—'
		},
		{
			title: m.semos_workspace_recent(),
			icon: Activity,
			items: recent,
			empty: 'No recent activity.'
		},
		{
			title: m.semos_workspace_alarms(),
			icon: AlertTriangle,
			items: alarms,
			empty: 'No alarms.'
		}
	]);
</script>

<svelte:head>
	<title>{cfg.workspace.banner_title} — {cfg.branding.site_name}</title>
</svelte:head>

<!-- Workspace banner -->
<section class="relative overflow-hidden border-b border-border/70">
	<img
		src={cfg.workspace.banner_image}
		alt=""
		class="absolute inset-0 h-full w-full object-cover opacity-10"
	/>
	<div class="absolute inset-0 bg-background/90"></div>
	<div class="relative mx-auto max-w-6xl px-6 py-10 md:py-14">
		<h1 class="text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
			{cfg.workspace.banner_title}
		</h1>
		<p class="mt-2 max-w-[60ch] text-muted-foreground">{cfg.workspace.banner_subtitle}</p>
		{#if tenantError}
			<p class="mt-3 flex items-center gap-1.5 text-sm text-destructive">
				<AlertTriangle class="h-4 w-4 shrink-0" />
				{tenantError}
			</p>
		{/if}
	</div>
</section>

<div class="mx-auto max-w-6xl px-6 py-10 md:py-12">
	<!-- Announcements / Recent activities / Alarms -->
	<div class="grid gap-4 md:grid-cols-3">
		{#each feeds as block (block.title)}
			<div class="rounded-lg border border-border p-5">
				<div class="flex items-center justify-between">
					<h2
						class="flex items-center gap-1.5 text-xs font-semibold tracking-wide text-muted-foreground uppercase"
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
				<ul class="mt-3 space-y-2 text-sm">
					{#if block.items.length === 0}
						<li class="text-muted-foreground">{block.empty}</li>
					{:else}
						{#each block.items as item (item)}
							<li>{item}</li>
						{/each}
					{/if}
				</ul>
			</div>
		{/each}
	</div>

	<!-- Apps grid: large rounded rectangles -->
	<h2 class="mt-10 text-lg font-semibold tracking-tight text-foreground">
		{m.semos_workspace_apps()}
	</h2>
	<div class="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
		{#each cfg.workspace.apps as app (app.name)}
			{@const Icon = icons[app.icon] ?? LayoutGrid}
			<a
				href={app.href}
				class="group flex flex-col gap-3 rounded-xl border border-border p-6 transition-colors duration-200 hover:border-foreground/20 hover:bg-accent/60 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
			>
				<div class="flex items-start justify-between">
					<span class="inline-flex rounded-lg bg-primary/10 p-2.5 text-primary">
						<Icon class="h-6 w-6" />
					</span>
					<ArrowUpRight
						class="h-4 w-4 text-muted-foreground/60 transition-transform duration-200 group-hover:translate-x-0.5 group-hover:-translate-y-0.5"
					/>
				</div>
				<span class="text-base font-semibold tracking-tight text-foreground">{app.name}</span>
				<span class="text-sm text-muted-foreground">{app.description}</span>
			</a>
		{/each}
	</div>
</div>

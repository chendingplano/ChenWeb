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

	const icons: Record<string, typeof LayoutGrid> = {
		database: Database,
		'message-circle': MessageCircle,
		search: Search,
		'file-check': FileCheck,
		workflow: Workflow,
		bot: Bot
	};

	const announcements = ['Welcome to your SemOS workspace.'];
	const recent: string[] = [];
	const alarms: string[] = [];

	const feeds = $derived([
		{ title: m.semos_workspace_announcements(), icon: Megaphone, items: announcements, empty: '—', accent: '#6b7aff' },
		{ title: m.semos_workspace_recent(), icon: Activity, items: recent, empty: 'No recent activity.', accent: '#3b82f6' },
		{ title: m.semos_workspace_alarms(), icon: AlertTriangle, items: alarms, empty: 'No alarms.', accent: '#f59e0b' }
	]);

	function reveal(node: HTMLElement) {
		const observer = new IntersectionObserver(
			(entries) => {
				for (const entry of entries) {
					if (entry.isIntersecting) { entry.target.classList.add('is-visible'); observer.unobserve(entry.target); }
				}
			},
			{ threshold: 0.1, rootMargin: '0px 0px -10% 0px' }
		);
		observer.observe(node);
		return { destroy() { observer.disconnect(); } };
	}
</script>

<svelte:head>
	<title>{cfg.workspace.banner_title} — {cfg.branding.site_name}</title>
</svelte:head>

<!-- ═══════════════════════════════════════════════
     BANNER — dark angle-overlay hero band
     ═══════════════════════════════════════════════ -->
<section class="relative overflow-hidden bg-[#080b14]">
	<div class="pointer-events-none absolute inset-0 mix-blend-overlay opacity-[0.04]"
		style="background-image: repeating-linear-gradient(0deg, transparent, transparent 2px, #fff 2px, #fff 3px), repeating-linear-gradient(90deg, transparent, transparent 2px, #fff 2px, #fff 3px); background-size: 60px 60px;">
	</div>
	<div class="pointer-events-none absolute -top-24 left-1/2 h-[30rem] w-[30rem] -translate-x-1/2 rounded-full opacity-[0.06] blur-[100px]"
		style="background: radial-gradient(circle, #6b7aff 0%, transparent 65%)">
	</div>
	<img src={cfg.workspace.banner_image} alt=""
		class="absolute inset-0 h-full w-full object-cover opacity-[0.4]"
	/>

	<div class="relative mx-auto max-w-7xl px-6 py-20 md:py-28">
		<div use:reveal class="reveal max-w-2xl">
			<h1 class="text-[clamp(1.75rem,3vw+0.5rem,2.75rem)] font-bold leading-[1.08] tracking-tight text-white">
				{cfg.workspace.banner_title}
			</h1>
			<p class="mt-4 max-w-[56ch] text-white/50">
				{cfg.workspace.banner_subtitle}
			</p>
			{#if tenantError}
				<p class="mt-5 flex items-center gap-1.5 text-sm text-red-400">
					<AlertTriangle class="h-4 w-4 shrink-0" />
					{tenantError}
				</p>
			{/if}
		</div>
	</div>

	<div class="absolute -bottom-px left-0 right-0 h-12 bg-[#f4f2ed] dark:bg-[#0a0d18]"
		style="clip-path: polygon(0 60%, 100% 0, 100% 100%, 0 100%);">
	</div>
</section>

<!-- ═══════════════════════════════════════════════
     FEED CARDS — glassy dark panels on a warmer
     tone band
     ═══════════════════════════════════════════════ -->
<section class="relative bg-[#f4f2ed] dark:bg-[#0a0d18]">
	<div class="mx-auto max-w-7xl px-6 py-14 md:py-20">
		<div use:reveal class="reveal grid gap-6 md:grid-cols-3">
			{#each feeds as block (block.title)}
				<div
					class="group relative overflow-hidden rounded-2xl bg-white p-6 shadow-md transition-all duration-200 hover:shadow-lg dark:bg-[#131726]"
				>
					<!-- Coloured top edge strip -->
					<div class="absolute top-0 left-4 right-4 h-1 rounded-full"
						style="background: {block.accent}">
					</div>
					<div class="flex items-start justify-between pt-2">
						<h2 class="flex items-center gap-2 text-xs font-bold tracking-wider uppercase"
							style="color: {block.accent}">
							<block.icon class="h-3.5 w-3.5" />
							{block.title}
						</h2>
						{#if block.items.length > 0}
							<span class="rounded-full bg-[#6b7aff]/10 px-2 py-0.5 text-[11px] font-bold tabular-nums text-[#6b7aff]">
								{block.items.length}
							</span>
						{/if}
					</div>
					<ul class="mt-4 space-y-2 text-sm leading-relaxed text-[#6b6b6b] dark:text-[#9a9aa0]">
						{#if block.items.length === 0}
							<li class="italic text-[#6b6b6b]/50 dark:text-[#9a9aa0]/40">{block.empty}</li>
						{:else}
							{#each block.items as item (item)}
								<li>{item}</li>
							{/each}
						{/if}
					</ul>
				</div>
			{/each}
		</div>

		<!-- ═══════════════════════════════════════════
		     APPS GRID — dimensional cards on glass
		     ═══════════════════════════════════════════ -->
		<div use:reveal class="reveal mt-14">
			<div class="mb-8 flex items-center gap-4">
				<h2 class="text-lg font-bold tracking-tight text-[#1a1a1a] dark:text-[#e8e7e4]">
					{m.semos_workspace_apps()}
				</h2>
				<span class="h-px flex-1 bg-[#6b7aff]/20"></span>
			</div>
			<div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
				{#each cfg.workspace.apps as app (app.name)}
					{@const Icon = icons[app.icon] ?? LayoutGrid}
					<a
						href={app.href}
						class="group relative overflow-hidden rounded-2xl border border-white/60 bg-white p-6 shadow-md transition-all duration-300 hover:-translate-y-0.5 hover:shadow-xl dark:border-[#1a1f30] dark:bg-[#131726]"
					>
						<div class="pointer-events-none absolute -right-6 -top-6 h-20 w-20 rounded-full opacity-10 blur-2xl"
							style="background: radial-gradient(circle, #6b7aff 0%, transparent 70%)">
						</div>
						<div class="flex items-start justify-between">
							<div class="inline-flex rounded-xl bg-[#6b7aff]/10 p-3 text-[#6b7aff] ring-1 ring-[#6b7aff]/10">
								<Icon class="h-6 w-6" />
							</div>
							<ArrowUpRight class="mt-1 h-4 w-4 text-[#6b7aff]/40 transition-all duration-200 group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:text-[#6b7aff]" />
						</div>
						<h3 class="mt-5 font-bold tracking-tight text-[#1a1a1a] dark:text-[#e8e7e4]">{app.name}</h3>
						<p class="mt-1.5 text-sm leading-relaxed text-[#6b6b6b] dark:text-[#9a9aa0]">{app.description}</p>
					</a>
				{/each}
			</div>
		</div>
	</div>
</section>

<style>
	.reveal {
		opacity: 0;
		transform: translateY(0.75rem);
		transition: opacity 0.5s cubic-bezier(0.16, 1, 0.3, 1),
			transform 0.5s cubic-bezier(0.16, 1, 0.3, 1);
	}
	.reveal:global(.is-visible) { opacity: 1; transform: translateY(0); }
	@media (prefers-reduced-motion: reduce) {
		.reveal { opacity: 1; transform: none; transition: none; }
	}
</style>

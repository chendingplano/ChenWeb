<script lang="ts">
	import { page } from '$app/state';
	import { m } from '$lib/paraglide/messages.js';
	import { fetchTenantSiteConfig, type SiteConfig } from '$lib/services/siteConfigService';
	import Ornament from '../components/Ornament.svelte';
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

	// Recent activity and alarms have no backing endpoint yet. They render an
	// empty state rather than sample content: ADR 2026071102 records a prior
	// pass that shipped invented figures, and demo data that reads as real is
	// exactly that mistake. Wire these to a real feed when one exists.
	const recentActivity: string[] = [];
	const alarms: string[] = [];

	const feeds = $derived([
		{
			title: m.semos_workspace_announcements(),
			icon: Megaphone,
			items: cfg.workspace.announcements ?? [],
			empty: m.semos_workspace_no_announcements(),
			alert: false
		},
		{
			title: m.semos_workspace_recent(),
			icon: Activity,
			items: recentActivity,
			empty: m.semos_workspace_no_activity(),
			alert: false
		},
		{
			title: m.semos_workspace_alarms(),
			icon: AlertTriangle,
			items: alarms,
			empty: m.semos_workspace_no_alarms(),
			// An alarm count is a signal, not decoration — it is the one thing on
			// this page allowed to break the bronze palette when it is non-zero.
			alert: true
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

<!-- ═════════════════════════════════════════════════
     BANNER — the main hero's paper-veil treatment at
     app-shell height. Marketing pages breathe; this
     one gets you to your apps (ADR 2026071102).
     ═════════════════════════════════════════════════ -->
<section class="relative overflow-hidden">
	<img src={cfg.workspace.banner_image} alt="" class="absolute inset-0 h-full w-full object-cover" />
	<div class="absolute inset-0 bg-[#faf9f7]/70 dark:bg-[#101216]/70"></div>
	<div
		class="absolute inset-0 bg-gradient-to-r from-[#faf9f7] via-[#faf9f7]/50 to-transparent dark:from-[#101216] dark:via-[#101216]/50"
	></div>

	<div class="relative mx-auto max-w-7xl px-6 py-14 md:py-16">
		<div use:reveal class="reveal max-w-2xl">
			<div class="flex items-center gap-3">
				<span class="inline-block h-1.5 w-1.5 rotate-45 bg-[#b08d57]"></span>
				<span class="text-xs font-bold tracking-[0.22em] uppercase text-[#6f6c66] dark:text-[#a5a29b]">
					{cfg.workspace.kicker}
				</span>
			</div>

			<h1
				class="mt-5 text-[clamp(1.9rem,3vw+0.6rem,2.75rem)] font-bold leading-[1.1] tracking-tight text-[#17181c] dark:text-[#e9e7e2]"
			>
				{cfg.workspace.banner_title}
			</h1>

			<p class="mt-4 max-w-[56ch] leading-relaxed text-[#6f6c66] dark:text-[#a5a29b]">
				{cfg.workspace.banner_subtitle}
			</p>

			{#if tenantError}
				<p class="mt-5 flex items-center gap-1.5 text-sm text-[#b4462f] dark:text-[#e08a76]">
					<AlertTriangle class="h-4 w-4 shrink-0" />
					{tenantError}
				</p>
			{/if}
		</div>
	</div>
</section>

<div use:reveal class="reveal py-10 md:py-12">
	<Ornament />
</div>

<!-- ═════════════════════════════════════════════════
     FEEDS — the main page's card material: white-to-
     paper gradient, white top bevel, layered shadow.
     ═════════════════════════════════════════════════ -->
<section class="relative">
	<div class="mx-auto max-w-7xl px-6">
		<div use:reveal class="reveal grid gap-6 md:grid-cols-3">
			{#each feeds as block, i (block.title)}
				<div
					style="transition-delay: {i * 60}ms"
					class="flex flex-col rounded-2xl border-t border-white bg-gradient-to-b from-white to-[#faf8f4] p-6 shadow-[0_1px_2px_rgba(23,24,28,0.06),0_3px_6px_rgba(23,24,28,0.05),0_12px_28px_rgba(23,24,28,0.09)] dark:border-white/10 dark:from-[#1c2029] dark:to-[#171a21] dark:shadow-[0_1px_2px_rgba(0,0,0,0.4),0_12px_28px_rgba(0,0,0,0.5)]"
				>
					<div class="flex items-start justify-between gap-2">
						<h2
							class="flex items-center gap-2 text-xs font-bold tracking-[0.18em] uppercase text-[#b08d57]"
						>
							<block.icon class="h-3.5 w-3.5" />
							{block.title}
						</h2>
						{#if block.items.length > 0}
							<span
								class="rounded-full px-2 py-0.5 text-[11px] font-bold tabular-nums {block.alert
									? 'bg-[#b4462f]/12 text-[#b4462f] dark:bg-[#e08a76]/15 dark:text-[#e08a76]'
									: 'bg-[#b08d57]/12 text-[#b08d57]'}"
							>
								{block.items.length}
							</span>
						{/if}
					</div>

					{#if block.items.length === 0}
						<!-- Designed empty state: a quiet bronze mark, not an apology. -->
						<div class="flex flex-1 flex-col items-center justify-center gap-2.5 py-8">
							<span
								class="inline-block h-2 w-2 rotate-45 bg-[#b08d57]/25 dark:bg-[#b08d57]/35"
								aria-hidden="true"
							></span>
							<p class="text-center text-sm text-[#6f6c66]/70 dark:text-[#a5a29b]/60">
								{block.empty}
							</p>
						</div>
					{:else}
						<ul class="mt-5 space-y-2.5 text-sm leading-relaxed text-[#6f6c66] dark:text-[#a5a29b]">
							{#each block.items as item (item)}
								<li class="flex gap-2.5">
									<span
										class="mt-[0.45rem] inline-block h-1 w-1 shrink-0 rounded-full bg-[#b08d57]/50"
										aria-hidden="true"
									></span>
									<span>{item}</span>
								</li>
							{/each}
						</ul>
					{/if}
				</div>
			{/each}
		</div>
	</div>
</section>

<div use:reveal class="reveal py-10 md:py-12">
	<Ornament />
</div>

<!-- ═════════════════════════════════════════════════
     APPS — the main page's feature-card idiom exactly:
     bronze icon coin, hover lift, revealed arrow.
     ═════════════════════════════════════════════════ -->
<section class="relative bg-[#f3f1ec] dark:bg-[#15181e]">
	<div class="mx-auto max-w-7xl px-6 py-16 md:py-20">
		<div use:reveal class="reveal mb-10 flex items-center gap-3">
			<span class="inline-block h-1.5 w-1.5 rotate-45 bg-[#b08d57]"></span>
			<h2 class="text-xs font-bold tracking-[0.22em] uppercase text-[#6f6c66] dark:text-[#a5a29b]">
				{m.semos_workspace_apps()}
			</h2>
		</div>

		<div use:reveal class="reveal grid gap-7 sm:grid-cols-2 lg:grid-cols-3">
			{#each cfg.workspace.apps as app, i (app.name)}
				{@const Icon = icons[app.icon] ?? LayoutGrid}
				<a
					href={app.href}
					style="transition-delay: {i * 60}ms"
					class="group relative flex flex-col rounded-2xl border-t border-white bg-gradient-to-b from-white to-[#faf8f4] p-7 shadow-[0_1px_2px_rgba(23,24,28,0.06),0_3px_6px_rgba(23,24,28,0.05),0_12px_28px_rgba(23,24,28,0.09)] transition-all duration-300 hover:-translate-y-1.5 hover:shadow-[0_2px_4px_rgba(23,24,28,0.07),0_6px_12px_rgba(23,24,28,0.07),0_24px_48px_rgba(23,24,28,0.14)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#b08d57]/50 dark:border-white/10 dark:from-[#1c2029] dark:to-[#171a21] dark:shadow-[0_1px_2px_rgba(0,0,0,0.4),0_12px_28px_rgba(0,0,0,0.5)]"
				>
					<div
						class="inline-flex w-fit rounded-full bg-gradient-to-b from-[#f6f4ef] to-[#e9e5dc] p-3.5 text-[#b08d57] shadow-[inset_0_1px_1px_rgba(255,255,255,0.9),0_2px_6px_rgba(23,24,28,0.12)] transition-transform duration-300 group-hover:scale-105 dark:from-[#252a35] dark:to-[#1c2029] dark:shadow-[inset_0_1px_1px_rgba(255,255,255,0.08),0_2px_6px_rgba(0,0,0,0.5)]"
					>
						<Icon class="h-6 w-6" />
					</div>
					<div class="mt-6 flex items-start justify-between gap-2">
						<h3 class="font-bold tracking-tight text-[#17181c] dark:text-[#e9e7e2]">{app.name}</h3>
						<ArrowUpRight
							class="mt-0.5 h-4 w-4 shrink-0 text-[#b08d57]/0 transition-all duration-300 group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:text-[#b08d57]"
						/>
					</div>
					<p class="mt-2.5 text-sm leading-relaxed text-[#6f6c66] dark:text-[#a5a29b]">
						{app.description}
					</p>
				</a>
			{/each}
		</div>
	</div>
</section>

<style>
	.reveal {
		opacity: 0;
		transform: translateY(0.75rem);
		transition:
			opacity 0.5s cubic-bezier(0.16, 1, 0.3, 1),
			transform 0.5s cubic-bezier(0.16, 1, 0.3, 1);
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

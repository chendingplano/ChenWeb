<script lang="ts">
	import { page } from '$app/state';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import { fetchTenantSiteConfig, type SiteConfig } from '$lib/services/siteConfigService';
	import Ornament from '../components/Ornament.svelte';
	import { ArrowUpRight, AlertTriangle } from '@lucide/svelte';

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

	// Stacked headline, same treatment as the About frontispiece: split on
	// ASCII and CJK commas so tenant copy in either language stacks cleanly.
	const titleLines = $derived(
		cfg.workspace.banner_title
			.split(/([,，]\s*)/)
			.reduce<string[]>((lines, part, i) => {
				if (i % 2 === 0) lines.push(part);
				else lines[lines.length - 1] += part.trim();
				return lines;
			}, [])
			.filter(Boolean)
	);

	// Masthead dateline, like a broadsheet: real date, tenant's locale.
	const dateline = new Intl.DateTimeFormat(getLocale() === 'zh-cn' ? 'zh-CN' : 'en-US', {
		dateStyle: 'full'
	}).format(new Date());

	const announcements = $derived(cfg.workspace.announcements ?? []);

	// Recent activity and alarms have no backing endpoint yet. They render an
	// empty state rather than sample content: ADR 2026071102 records a prior
	// pass that shipped invented figures, and demo data that reads as real is
	// exactly that mistake. Wire these to a real feed when one exists.
	const recentActivity: string[] = [];
	const alarms: string[] = [];

	const deskColumns = $derived([
		{
			title: m.semos_workspace_recent(),
			items: recentActivity,
			empty: m.semos_workspace_no_activity(),
			alert: false
		},
		{
			title: m.semos_workspace_alarms(),
			items: alarms,
			empty: m.semos_workspace_no_alarms(),
			// An alarm count is a signal, not decoration: it is the one thing on
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
     MASTHEAD — the banner image stays, but set like a
     broadsheet: kicker and dateline share one rule,
     the title stacks like the About frontispiece, and
     the site's diamond watermark hangs at the right.
     ═════════════════════════════════════════════════ -->
<section class="relative overflow-hidden">
	<img src={cfg.workspace.banner_image} alt="" class="absolute inset-0 h-full w-full object-cover" />
	<div class="absolute inset-0 bg-[#faf9f7]/70 dark:bg-[#101216]/70"></div>
	<div
		class="absolute inset-0 bg-gradient-to-r from-[#faf9f7] via-[#faf9f7]/55 to-transparent dark:from-[#101216] dark:via-[#101216]/55"
	></div>

	<!-- Watermark: the ornament's diamond at architectural scale, kin to About -->
	<div
		aria-hidden="true"
		class="pointer-events-none absolute top-1/2 right-[-4rem] hidden -translate-y-1/2 lg:block"
	>
		<div class="h-64 w-64 rotate-45 border border-[#b08d57]/30"></div>
		<div
			class="absolute top-1/2 left-1/2 h-40 w-40 -translate-x-1/2 -translate-y-1/2 rotate-45 border border-[#b08d57]/20"
		></div>
		<div
			class="absolute top-1/2 left-1/2 h-3 w-3 -translate-x-1/2 -translate-y-1/2 rotate-45 bg-[#b08d57]/60"
		></div>
	</div>

	<div class="relative mx-auto max-w-7xl px-6 py-16 md:py-20">
		<div use:reveal class="reveal">
			<div
				class="flex items-baseline justify-between gap-6 border-b border-[#17181c]/15 pb-4 dark:border-white/15"
			>
				<div class="flex items-center gap-3">
					<span class="inline-block h-1.5 w-1.5 rotate-45 bg-[#b08d57]"></span>
					<span
						class="text-xs font-bold tracking-[0.22em] uppercase text-[#6f6c66] dark:text-[#a5a29b]"
					>
						{cfg.workspace.kicker}
					</span>
				</div>
				<span class="text-xs tracking-[0.06em] tabular-nums text-[#6f6c66] dark:text-[#a5a29b]">
					{dateline}
				</span>
			</div>

			<h1
				class="mt-7 max-w-3xl text-[clamp(2rem,3.2vw+0.6rem,3rem)] font-bold leading-[1.14] tracking-tight text-[#17181c] dark:text-[#e9e7e2]"
			>
				{#each titleLines as line (line)}
					<span class="block">{line}</span>
				{/each}
			</h1>

			<p class="mt-4 max-w-[56ch] leading-[1.9] text-[#6f6c66] dark:text-[#a5a29b]">
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

<!-- ═════════════════════════════════════════════════
     BULLETIN — announcements as a ruled notice strip,
     not a card: kicker in the margin column (the About
     story spread's layout), hairline-ruled lines with
     diamond markers beside it.
     ═════════════════════════════════════════════════ -->
<section class="relative">
	<div class="mx-auto max-w-7xl px-6 pt-12 md:pt-16">
		<div use:reveal class="reveal grid gap-6 md:grid-cols-[200px_1fr] md:gap-16 lg:grid-cols-[240px_1fr]">
			<div class="flex items-center gap-3 self-start md:pt-4">
				<span class="inline-block h-1.5 w-1.5 rotate-45 bg-[#b08d57]"></span>
				<h2 class="text-xs font-bold tracking-[0.22em] uppercase text-[#b08d57]">
					{m.semos_workspace_announcements()}
				</h2>
				{#if announcements.length > 0}
					<span class="text-xs font-bold tabular-nums text-[#b08d57]/60">
						{String(announcements.length).padStart(2, '0')}
					</span>
				{/if}
			</div>

			{#if announcements.length === 0}
				<p
					class="flex items-center gap-3 border-y border-[#17181c]/10 py-4 text-sm text-[#6f6c66]/70 dark:border-white/10 dark:text-[#a5a29b]/60"
				>
					<span
						class="inline-block h-1.5 w-1.5 shrink-0 rotate-45 bg-[#b08d57]/30"
						aria-hidden="true"
					></span>
					{m.semos_workspace_no_announcements()}
				</p>
			{:else}
				<ul class="border-t border-[#17181c]/10 dark:border-white/10">
					{#each announcements as item (item)}
						<li
							class="flex gap-3 border-b border-[#17181c]/10 py-4 leading-[1.8] text-[#3d3c38] dark:border-white/10 dark:text-[#c6c3bc]"
						>
							<span
								class="mt-[0.55rem] inline-block h-1.5 w-1.5 shrink-0 rotate-45 bg-[#b08d57]"
								aria-hidden="true"
							></span>
							<span>{item}</span>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	</div>
</section>

<div use:reveal class="reveal py-12 md:py-16">
	<Ornament />
</div>

<!-- ═════════════════════════════════════════════════
     APPS — the page's one drenched moment: paper
     inverts to ink and the apps read as the volume's
     index. No cards, no icon coins: bronze numerals,
     hairline-ruled cells, an arrow that answers hover.
     Kin to the About manifesto, set as a two-column
     index rather than manifesto rows.
     ═════════════════════════════════════════════════ -->
<section class="relative bg-[#17181c] dark:bg-[#0b0d10]">
	<div class="mx-auto max-w-7xl px-6 py-16 md:py-24">
		<div use:reveal class="reveal flex items-baseline justify-between gap-6">
			<div class="flex items-center gap-3">
				<span class="inline-block h-1.5 w-1.5 rotate-45 bg-[#b08d57]"></span>
				<h2 class="text-xs font-bold tracking-[0.22em] uppercase text-[#b08d57]">
					{m.semos_workspace_apps()}
				</h2>
			</div>
			<span class="text-xs font-bold tabular-nums text-[#b08d57]/60">
				{String(cfg.workspace.apps.length).padStart(2, '0')}
			</span>
		</div>

		<div use:reveal class="reveal mt-10 grid border-t border-white/12 sm:grid-cols-2">
			{#each cfg.workspace.apps as app, i (app.name)}
				<a
					href={app.href}
					class="group relative grid grid-cols-[3.25rem_1fr_auto] items-baseline gap-y-2.5 border-b border-white/12 px-2 py-8 transition-colors duration-200 hover:bg-white/[0.04] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[#b08d57]/60 sm:px-8 sm:py-10 sm:odd:border-r"
				>
					<span
						class="text-lg font-bold tabular-nums text-[#b08d57]/50 transition-colors duration-200 group-hover:text-[#b08d57]"
					>
						{String(i + 1).padStart(2, '0')}
					</span>
					<h3 class="text-xl font-bold leading-snug tracking-tight text-[#f5f3ef]">
						{app.name}
					</h3>
					<ArrowUpRight
						class="h-5 w-5 shrink-0 self-center text-[#b08d57] opacity-0 transition-all duration-200 group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:opacity-100"
					/>
					<p class="col-start-2 max-w-[46ch] text-sm leading-[1.8] text-[#a5a29b]">
						{app.description}
					</p>
				</a>
			{/each}
		</div>
	</div>
</section>

<!-- ═════════════════════════════════════════════════
     DESK — back on paper: activity and alarms as two
     ruled columns, same notice vocabulary as the
     bulletin. Empty states stay designed: a quiet
     bronze mark, not an apology.
     ═════════════════════════════════════════════════ -->
<section class="relative">
	<div class="mx-auto max-w-7xl px-6 py-16 md:py-24">
		<div use:reveal class="reveal grid gap-12 md:grid-cols-2 md:gap-16">
			{#each deskColumns as col (col.title)}
				<div>
					<div class="flex items-center gap-3">
						<span class="inline-block h-1.5 w-1.5 rotate-45 bg-[#b08d57]"></span>
						<h2 class="text-xs font-bold tracking-[0.22em] uppercase text-[#b08d57]">
							{col.title}
						</h2>
						{#if col.items.length > 0}
							<span
								class="rounded-full px-2 py-0.5 text-[11px] font-bold tabular-nums {col.alert
									? 'bg-[#b4462f]/12 text-[#b4462f] dark:bg-[#e08a76]/15 dark:text-[#e08a76]'
									: 'bg-[#b08d57]/12 text-[#b08d57]'}"
							>
								{col.items.length}
							</span>
						{/if}
					</div>

					{#if col.items.length === 0}
						<div
							class="mt-5 flex flex-col items-center gap-2.5 border-y border-[#17181c]/10 py-10 dark:border-white/10"
						>
							<span
								class="inline-block h-2 w-2 rotate-45 bg-[#b08d57]/25 dark:bg-[#b08d57]/35"
								aria-hidden="true"
							></span>
							<p class="text-center text-sm text-[#6f6c66]/70 dark:text-[#a5a29b]/60">
								{col.empty}
							</p>
						</div>
					{:else}
						<ul class="mt-5 border-t border-[#17181c]/10 text-sm dark:border-white/10">
							{#each col.items as item (item)}
								<li
									class="flex gap-3 border-b border-[#17181c]/10 py-3.5 leading-[1.8] text-[#6f6c66] dark:border-white/10 dark:text-[#a5a29b]"
								>
									<span
										class="mt-[0.5rem] inline-block h-1 w-1 shrink-0 rounded-full bg-[#b08d57]/50"
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

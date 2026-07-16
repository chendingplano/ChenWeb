<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import {
		Database,
		MessageCircle,
		Search,
		Bot,
		Sparkles,
		ArrowRight,
		ArrowUpRight
	} from '@lucide/svelte';
	import { appAuthStore } from '@chendingplano/shared';
	import Ornament from './components/Ornament.svelte';
	import LoginPanel from '$lib/components/login-01.svelte';
	import { loginPrompt } from './loginPrompt.svelte';

	let { data } = $props();
	const cfg = $derived(data.siteConfig);

	function handleGetStarted(event: MouseEvent) {
		if (!appAuthStore.getIsLoggedIn()) {
			event.preventDefault();
			loginPrompt.show();
		}
	}

	function handleGetStartedFromBottom(event: MouseEvent) {
		if (!appAuthStore.getIsLoggedIn()) {
			event.preventDefault();
			window.scrollTo({ top: 0, behavior: 'smooth' });
			loginPrompt.show();
		}
	}

	const featureIcons: Record<string, typeof Database> = {
		knowledge_base: Database,
		chat: MessageCircle,
		search: Search,
		app_dev: Bot
	};

	// Stack the slogan: each comma-separated phrase becomes its own line,
	// giving the stacked multi-line headline treatment. Falls back to a
	// single line when the slogan has no commas.
	const sloganLines = $derived(
		cfg.hero.slogan
			.split(/,\s*/)
			.map((s: string, i: number, arr: string[]) => (i < arr.length - 1 ? s + ',' : s))
	);

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
			{ threshold: 0.15, rootMargin: '0px 0px -8% 0px' }
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
	<title>{cfg.branding.site_name}</title>
	<meta name="description" content={cfg.hero.subtitle} />
</svelte:head>

<!-- ═════════════════════════════════════════════════
     HERO — light and airy. The image shows at nearly
     full strength; a soft paper wash sweeps in from
     the left only where the text needs contrast.
     ═════════════════════════════════════════════════ -->
<section class="relative overflow-hidden">
	<img src={cfg.hero.image} alt="" class="absolute inset-0 h-full w-full object-cover" />
	<!-- Full-bleed pale veil: keeps the entire image light, mirai-style -->
	<div class="absolute inset-0 bg-[#faf9f7]/70 dark:bg-[#101216]/70"></div>
	<!-- Extra wash behind the text column only -->
	<div
		class="absolute inset-0 bg-gradient-to-r from-[#faf9f7] via-[#faf9f7]/50 to-transparent dark:from-[#101216] dark:via-[#101216]/50"
	></div>

	<div class="relative mx-auto max-w-7xl px-6 py-28 md:py-36 lg:py-44">
		<div
			class={$appAuthStore.isLoggedIn || loginPrompt.open
				? ''
				: 'grid gap-12 lg:grid-cols-[1fr_auto] lg:items-center'}
		>
			<div use:reveal class="reveal max-w-2xl">
				<!-- Kicker with bronze diamond ornament -->
				<div class="flex items-center gap-3">
					<span class="inline-block h-1.5 w-1.5 rotate-45 bg-[#b08d57]"></span>
					<span
						class="text-xs font-bold tracking-[0.22em] text-[#6f6c66] uppercase dark:text-[#a5a29b]"
					>
						{cfg.hero.kicker}
					</span>
				</div>

				<!-- Stacked headline -->
				<h1
					class="mt-7 text-[clamp(2.6rem,4.5vw+1rem,4.5rem)] leading-[1.08] font-bold tracking-tight text-[#17181c] dark:text-[#e9e7e2]"
				>
					{#each sloganLines as line (line)}
						<span class="block">{line}</span>
					{/each}
				</h1>

				<p
					class="mt-7 max-w-[52ch] text-base leading-relaxed text-[#6f6c66] md:text-lg dark:text-[#a5a29b]"
				>
					{cfg.hero.subtitle}
				</p>

				<div class="mt-10 flex flex-wrap items-center gap-4">
					<a
						href={cfg.hero.cta_primary_href}
						onclick={handleGetStarted}
						class="group inline-flex items-center gap-2 rounded-lg bg-[#17181c] px-7 py-3.5 text-sm font-semibold text-white shadow-[0_2px_4px_rgba(23,24,28,0.18),0_10px_28px_rgba(23,24,28,0.22)] transition-all duration-200 hover:-translate-y-0.5 hover:shadow-[0_4px_8px_rgba(23,24,28,0.2),0_16px_36px_rgba(23,24,28,0.26)] focus-visible:ring-2 focus-visible:ring-[#17181c]/40 focus-visible:outline-none active:translate-y-0 dark:bg-white dark:text-[#17181c]"
					>
						{m.semos_get_started()}
						<ArrowRight
							class="h-4 w-4 transition-transform duration-200 group-hover:translate-x-0.5"
						/>
					</a>
				</div>
			</div>

			{#if !$appAuthStore.isLoggedIn && !loginPrompt.open}
				<div class="flex justify-center lg:justify-end lg:translate-x-[200px]">
					<LoginPanel />
				</div>
			{/if}
		</div>
	</div>
</section>

<!-- ═════════════════════════════════════════════════
     HIGHLIGHTS — airy alternating rows on paper,
     images in floating 3-D frames
     ═════════════════════════════════════════════════ -->
<section class="relative">
	<div class="mx-auto max-w-7xl px-6 pt-20 pb-8 md:pt-28">
		{#each cfg.highlights as h, i (h.title)}
			{#if i > 0}
				<div use:reveal class="reveal py-14 md:py-20">
					<Ornament class="py-2" />
				</div>
			{/if}
			<div
				use:reveal
				class="reveal grid items-center gap-10 md:grid-cols-2 md:gap-20 {i % 2 === 1
					? 'md:[&>*:first-child]:order-2'
					: ''}"
			>
				<!-- Text -->
				<div>
					<span class="text-xs font-bold tracking-[0.22em] text-[#b08d57] uppercase">
						{String(i + 1).padStart(2, '0')}
					</span>
					<h2
						class="mt-4 text-2xl leading-snug font-bold tracking-tight text-[#17181c] md:text-[2rem] md:leading-tight dark:text-[#e9e7e2]"
					>
						{h.title}
					</h2>
					<p class="mt-4 max-w-[48ch] leading-relaxed text-[#6f6c66] dark:text-[#a5a29b]">
						{h.description}
					</p>
				</div>

				<!-- Image in a floating 3-D frame: white mat + layered shadow + tilt -->
				<div class="group relative">
					<!-- Soft shadow puddle underneath -->
					<div
						class="absolute inset-x-8 -bottom-4 h-8 rounded-[50%] bg-[#17181c]/10 blur-xl transition-all duration-500 group-hover:inset-x-6 group-hover:bg-[#17181c]/14 dark:bg-black/40"
					></div>
					<div
						class="relative overflow-hidden rounded-xl border-t border-white bg-white p-2.5 shadow-[0_1px_2px_rgba(23,24,28,0.08),0_12px_32px_rgba(23,24,28,0.14)] transition-all duration-500 group-hover:shadow-[0_2px_4px_rgba(23,24,28,0.1),0_20px_48px_rgba(23,24,28,0.18)] dark:border-white/10 dark:bg-[#1a1d24]"
					>
						<img src={h.image} alt={h.title} class="aspect-video w-full rounded-lg object-cover" />
					</div>
				</div>
			</div>
		{/each}
	</div>
</section>

<!-- Ornament between highlights and features -->
<div use:reveal class="reveal py-12 md:py-16">
	<Ornament class="py-2" />
</div>

<!-- ═════════════════════════════════════════════════
     FEATURES — cards with real depth: raised surface,
     layered shadows, light top bevel, hover lift
     ═════════════════════════════════════════════════ -->
<section class="relative bg-[#f3f1ec] dark:bg-[#15181e]">
	<div class="mx-auto max-w-7xl px-6 py-24 md:py-28">
		<div use:reveal class="reveal mx-auto max-w-xl text-center">
			<div class="flex items-center justify-center gap-3">
				<span class="inline-block h-1.5 w-1.5 rotate-45 bg-[#b08d57]"></span>
				<span
					class="text-xs font-bold tracking-[0.22em] text-[#6f6c66] uppercase dark:text-[#a5a29b]"
				>
					{cfg.features_section.kicker}
				</span>
				<span class="inline-block h-1.5 w-1.5 rotate-45 bg-[#b08d57]"></span>
			</div>
			<h2 class="mt-5 text-3xl font-bold tracking-tight text-[#17181c] dark:text-[#e9e7e2]">
				{cfg.features_section.title}
			</h2>
			<p class="mt-4 text-[#6f6c66] dark:text-[#a5a29b]">
				{cfg.features_section.subtitle}
			</p>
		</div>

		<div class="mt-14 grid gap-7 sm:grid-cols-2 lg:grid-cols-4">
			{#each cfg.features as f, i (f.key)}
				{@const Icon = featureIcons[f.key] ?? Sparkles}
				<a
					href={f.href}
					style="transition-delay: {i * 60}ms"
					class="group relative flex flex-col rounded-2xl border-t border-white bg-gradient-to-b from-white to-[#faf8f4] p-7 shadow-[0_1px_2px_rgba(23,24,28,0.06),0_3px_6px_rgba(23,24,28,0.05),0_12px_28px_rgba(23,24,28,0.09)] transition-all duration-300 hover:-translate-y-1.5 hover:shadow-[0_2px_4px_rgba(23,24,28,0.07),0_6px_12px_rgba(23,24,28,0.07),0_24px_48px_rgba(23,24,28,0.14)] focus-visible:ring-2 focus-visible:ring-[#b08d57]/50 focus-visible:outline-none dark:border-white/10 dark:from-[#1c2029] dark:to-[#171a21] dark:shadow-[0_1px_2px_rgba(0,0,0,0.4),0_12px_28px_rgba(0,0,0,0.5)]"
				>
					<!-- Icon coin: raised disc with its own depth -->
					<div
						class="inline-flex w-fit rounded-full bg-gradient-to-b from-[#f6f4ef] to-[#e9e5dc] p-3.5 text-[#b08d57] shadow-[inset_0_1px_1px_rgba(255,255,255,0.9),0_2px_6px_rgba(23,24,28,0.12)] transition-transform duration-300 group-hover:scale-105 dark:from-[#252a35] dark:to-[#1c2029] dark:shadow-[inset_0_1px_1px_rgba(255,255,255,0.08),0_2px_6px_rgba(0,0,0,0.5)]"
					>
						<Icon class="h-6 w-6" />
					</div>
					<div class="mt-6 flex items-start justify-between gap-2">
						<h3 class="font-bold tracking-tight text-[#17181c] dark:text-[#e9e7e2]">{f.title}</h3>
						<ArrowUpRight
							class="mt-0.5 h-4 w-4 shrink-0 text-[#b08d57]/0 transition-all duration-300 group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:text-[#b08d57]"
						/>
					</div>
					<p class="mt-2.5 text-sm leading-relaxed text-[#6f6c66] dark:text-[#a5a29b]">
						{f.description}
					</p>
				</a>
			{/each}
		</div>
	</div>
</section>

<!-- ═════════════════════════════════════════════════
     CLOSING CTA — quiet, centered, paper again
     ═════════════════════════════════════════════════ -->
<section class="relative">
	<div class="mx-auto max-w-7xl px-6 py-24 text-center md:py-32">
		<div use:reveal class="reveal mx-auto max-w-xl">
			<Ornament class="py-2" />
			<h2
				class="mt-8 text-3xl font-bold tracking-tight text-[#17181c] md:text-4xl dark:text-[#e9e7e2]"
			>
				{cfg.cta.title}
			</h2>
			<p class="mt-4 text-[#6f6c66] dark:text-[#a5a29b]">
				{cfg.cta.subtitle}
			</p>
			<div class="mt-10 flex flex-wrap items-center justify-center gap-4">
				<a
					href={cfg.hero.cta_primary_href}
					onclick={handleGetStartedFromBottom}
					class="inline-flex items-center gap-2 rounded-lg bg-[#17181c] px-7 py-3.5 text-sm font-semibold text-white shadow-[0_2px_4px_rgba(23,24,28,0.18),0_10px_28px_rgba(23,24,28,0.22)] transition-all duration-200 hover:-translate-y-0.5 hover:shadow-[0_4px_8px_rgba(23,24,28,0.2),0_16px_36px_rgba(23,24,28,0.26)] active:translate-y-0 dark:bg-white dark:text-[#17181c]"
				>
					{m.semos_get_started()}
					<ArrowRight class="h-4 w-4" />
				</a>
				{#if !$appAuthStore.isLoggedIn}
					<button
						type="button"
						onclick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
						class="inline-flex items-center rounded-lg border border-[#17181c]/15 px-7 py-3.5 text-sm font-semibold text-[#17181c] transition-all duration-200 hover:-translate-y-0.5 hover:border-[#17181c]/30 dark:border-white/20 dark:text-white"
					>
						{m.semos_signup_login()}
					</button>
				{/if}
			</div>
		</div>
	</div>
</section>

<style>
	.reveal {
		opacity: 0;
		transform: translateY(1rem);
		transition:
			opacity 0.6s cubic-bezier(0.16, 1, 0.3, 1),
			transform 0.6s cubic-bezier(0.16, 1, 0.3, 1);
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

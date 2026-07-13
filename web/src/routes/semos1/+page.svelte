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

	let { data } = $props();
	const cfg = $derived(data.siteConfig);

	const featureIcons: Record<string, typeof Database> = {
		knowledge_base: Database,
		chat: MessageCircle,
		search: Search,
		app_dev: Bot
	};

	const stats = [
		{ label: 'Documents Processed', value: '127K+' },
		{ label: 'Metrics Extracted', value: '4.2M' },
		{ label: 'Enterprise Customers', value: '180+' },
		{ label: 'Provisions Analyzed', value: '960K' }
	];

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
			{ threshold: 0.15, rootMargin: '0px 0px -10% 0px' }
		);
		observer.observe(node);
		return { destroy() { observer.disconnect(); } };
	}
</script>

<svelte:head>
	<title>{cfg.branding.site_name}</title>
	<meta name="description" content={cfg.hero.subtitle} />
</svelte:head>

<!-- ═══════════════════════════════════════════════
     HERO — image-forward, gradient overlay at bottom
     ═══════════════════════════════════════════════ -->
<section class="relative overflow-hidden">
	<!-- Background image at full visibility -->
	<img src={cfg.hero.image} alt=""
		class="absolute inset-0 h-full w-full object-cover"
	/>
	<!-- Gradient overlay: darkens only at the bottom for text readability -->
	<div class="absolute inset-0 bg-gradient-to-b from-transparent via-[#080b14]/40 via-60% to-[#080b14]/85"></div>

	<div class="relative mx-auto max-w-7xl px-6 pt-28 pb-32 md:pt-40 md:pb-44 lg:pt-48 lg:pb-56">
		<div use:reveal class="reveal max-w-3xl">
			<span class="inline-block rounded-full border border-white/30 bg-white/10 px-4 py-1.5 text-xs font-semibold tracking-wider uppercase text-white/90">
				Knowledge Management · AI Powered
			</span>
			<h1 class="mt-8 text-[clamp(2.8rem,6vw+1rem,5.25rem)] font-bold leading-[1.02] tracking-tight text-white">
				{cfg.hero.slogan}
			</h1>
			<p class="mt-6 max-w-[54ch] text-base leading-relaxed text-white/70 md:text-lg">
				{cfg.hero.subtitle}
			</p>
			<div class="mt-10 flex flex-wrap items-center gap-4">
				<a
					href={cfg.hero.cta_primary_href}
					class="group inline-flex items-center gap-2 rounded-lg bg-white px-7 py-3.5 text-sm font-bold text-[#080b14] shadow-lg shadow-white/10 transition-all duration-200 hover:bg-white/90 hover:shadow-xl active:scale-[0.97] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/60"
				>
					{m.semos_get_started()}
					<ArrowRight class="h-4 w-4 transition-transform duration-200 group-hover:translate-x-1" />
				</a>
				<a
					href={cfg.hero.cta_secondary_href}
					class="inline-flex items-center rounded-lg border border-white/20 px-7 py-3.5 text-sm font-semibold text-white transition-all duration-200 hover:bg-white/10 active:scale-[0.97] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/40"
				>
					{m.semos_signup_login()}
				</a>
			</div>
		</div>
	</div>

	<!-- Diagonal cut at bottom of hero -->
	<div class="absolute -bottom-px left-0 right-0 h-16 bg-[#faf9f6] dark:bg-[#0c0f1a]"
		style="clip-path: polygon(0 60%, 100% 0, 100% 100%, 0 100%);">
	</div>
</section>

<!-- ═══════════════════════════════════════════════
     HIGHLIGHTS — alternating tone bands, each
     section a full-bleed stripe with its own mood
     ═══════════════════════════════════════════════ -->
{#each cfg.highlights as h, i (h.title)}
	{@const even = i % 2 === 0}
	{@const bgEven = 'bg-[#f4f2ed] dark:bg-[#131726]'}
	{@const bgOdd = 'bg-[#faf9f6] dark:bg-[#0a0d18]'}
	{@const accentGrad = even
		? 'linear-gradient(135deg, #6b7aff10, transparent 60%)'
		: 'linear-gradient(225deg, #6b7aff08, transparent 60%)'}

	<section class="relative overflow-hidden {even ? bgEven : bgOdd}">
		<!-- Section gradient wash -->
		<div class="pointer-events-none absolute inset-0" style="background: {accentGrad}"></div>

		<div use:reveal
			class="reveal relative mx-auto flex max-w-7xl flex-col items-center gap-12 px-6 py-20 md:flex-row md:py-28 {even ? '' : 'md:flex-row-reverse'}">
			<!-- Image block — larger, with decorative framing -->
			<div class="relative w-full md:w-3/5">
				<div class="relative overflow-hidden rounded-2xl shadow-xl">
					<img src={h.image} alt={h.title}
						class="aspect-[4/3] w-full object-cover transition-all duration-700 ease-out hover:scale-105"
					/>
				</div>
				<!-- Decorative floating ring -->
				<div class="pointer-events-none absolute -right-4 -bottom-4 h-20 w-20 rounded-full border border-[#6b7aff]/20 md:-right-6 md:-bottom-6 md:h-28 md:w-28"></div>
			</div>

			<!-- Text block -->
			<div class="w-full md:w-2/5">
				<span class="text-xs font-bold tracking-[0.2em] text-[#6b7aff] uppercase">
					{String(i + 1).padStart(2, '0')}
				</span>
				<h2 class="mt-4 text-2xl font-bold leading-snug tracking-tight text-[#1a1a1a] dark:text-[#e8e7e4] md:text-3xl md:leading-tight">
					{h.title}
				</h2>
				<p class="mt-4 leading-relaxed text-[#6b6b6b] dark:text-[#9a9aa0]">
					{h.description}
				</p>
			</div>
		</div>
	</section>
{/each}

<!-- ═══════════════════════════════════════════════
     STATS — full-bleed dark band with big numbers
     ═══════════════════════════════════════════════ -->
<section class="relative overflow-hidden bg-[#080b14]">
	<div class="pointer-events-none absolute inset-0 mix-blend-overlay opacity-[0.03]"
		style="background-image: repeating-linear-gradient(0deg, transparent, transparent 2px, #fff 2px, #fff 3px), repeating-linear-gradient(90deg, transparent, transparent 2px, #fff 2px, #fff 3px); background-size: 60px 60px;">
	</div>
	<div class="mx-auto max-w-7xl px-6 py-20 md:py-28">
		<div use:reveal class="reveal grid grid-cols-2 gap-12 md:grid-cols-4 md:gap-8">
			{#each stats as s (s.label)}
				<div class="text-center">
					<div class="text-4xl font-black tracking-tight text-white md:text-5xl">{s.value}</div>
					<div class="mt-2 text-sm font-medium text-white/50">{s.label}</div>
				</div>
			{/each}
		</div>
	</div>
</section>

<!-- ═══════════════════════════════════════════════
     FEATURES — bento-inspired grid
     ═══════════════════════════════════════════════ -->
<section class="relative bg-[#f4f2ed] dark:bg-[#131726]">
	<div class="mx-auto max-w-7xl px-6 py-24 md:py-32">
		<div use:reveal class="reveal max-w-2xl">
			<h2 class="text-3xl font-bold tracking-tight text-[#1a1a1a] dark:text-[#e8e7e4]">
				Everything you need
			</h2>
			<p class="mt-4 text-[#6b6b6b] dark:text-[#9a9aa0]">
				Four capabilities. One platform. Your knowledge, your way.
			</p>
		</div>

		<div class="mt-14 grid gap-6 sm:grid-cols-2">
			{#each cfg.features as f, i (f.key)}
				{@const Icon = featureIcons[f.key] ?? Sparkles}
				{@const cardBg = i === 0
					? 'bg-[#6b7aff] text-white'
					: 'bg-white dark:bg-[#1a1f30] text-[#1a1a1a] dark:text-[#e8e7e4]'}
				{@const descColor = i === 0
					? 'text-white/70'
					: 'text-[#6b6b6b] dark:text-[#9a9aa0]'}
				<a
					href={f.href}
					class="group relative overflow-hidden rounded-2xl p-8 shadow-lg transition-all duration-300 hover:-translate-y-1 hover:shadow-xl {cardBg}"
				>
					<div class="pointer-events-none absolute -right-8 -top-8 h-24 w-24 rounded-full opacity-20 blur-xl"
						style="background: radial-gradient(circle, {i === 0 ? '#fff' : '#6b7aff'} 0%, transparent 70%)">
					</div>
					<div class="flex items-start justify-between">
						<div class="inline-flex rounded-xl bg-white/15 p-3 ring-1 ring-white/10 backdrop-blur-sm">
							<Icon class="h-6 w-6" />
						</div>
						<ArrowUpRight class="h-5 w-5 opacity-60 transition-all duration-200 group-hover:translate-x-0.5 group-hover:-translate-y-0.5" />
					</div>
					<h3 class="mt-6 text-xl font-bold tracking-tight">{f.title}</h3>
					<p class="mt-2 leading-relaxed text-sm {descColor}">{f.description}</p>
				</a>
			{/each}
		</div>
	</div>
</section>

<!-- ═══════════════════════════════════════════════
     FINAL CTA — bookend dark band
     ═══════════════════════════════════════════════ -->
<section class="relative overflow-hidden bg-[#080b14]">
	<div class="pointer-events-none absolute inset-0 mix-blend-overlay opacity-[0.03]"
		style="background-image: repeating-linear-gradient(0deg, transparent, transparent 2px, #fff 2px, #fff 3px), repeating-linear-gradient(90deg, transparent, transparent 2px, #fff 2px, #fff 3px); background-size: 60px 60px;">
	</div>
	<div class="pointer-events-none absolute left-1/2 top-0 h-[30rem] w-[30rem] -translate-x-1/2 rounded-full opacity-[0.05] blur-[100px]"
		style="background: radial-gradient(circle, #6b7aff 0%, transparent 65%)">
	</div>
	<div class="relative mx-auto max-w-7xl px-6 py-24 text-center md:py-32">
		<div use:reveal class="reveal max-w-2xl mx-auto">
			<h2 class="text-3xl font-bold tracking-tight text-white md:text-4xl">
				Ready to get started?
			</h2>
			<p class="mt-4 text-white/60">
				Join teams that trust SemOS to manage, search, and act on their knowledge.
			</p>
			<div class="mt-10 flex flex-wrap items-center justify-center gap-4">
				<a
					href={cfg.hero.cta_primary_href}
					class="inline-flex items-center gap-2 rounded-lg bg-white px-7 py-3.5 text-sm font-bold text-[#080b14] shadow-lg transition-all duration-200 hover:bg-white/90 active:scale-[0.97] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/60"
				>
					{m.semos_get_started()}
					<ArrowRight class="h-4 w-4" />
				</a>
				<a
					href={cfg.hero.cta_secondary_href}
					class="inline-flex items-center rounded-lg border border-white/20 px-7 py-3.5 text-sm font-semibold text-white transition-all duration-200 hover:bg-white/10 active:scale-[0.97]"
				>
					{m.semos_signup_login()}
				</a>
			</div>
		</div>
	</div>
</section>

<style>
	.reveal {
		opacity: 0;
		transform: translateY(1.25rem);
		transition: opacity 0.6s cubic-bezier(0.16, 1, 0.3, 1),
				transform 0.6s cubic-bezier(0.16, 1, 0.3, 1);
	}
	.reveal:global(.is-visible) {
		opacity: 1;
		transform: translateY(0);
	}
	@media (prefers-reduced-motion: reduce) {
		.reveal { opacity: 1; transform: none; transition: none; }
	}
</style>

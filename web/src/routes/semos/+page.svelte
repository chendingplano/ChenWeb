<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { Database, MessageCircle, Search, Bot, Sparkles, ArrowRight, ArrowUpRight } from '@lucide/svelte';

	let { data } = $props();
	const cfg = $derived(data.siteConfig);

	const featureIcons: Record<string, typeof Database> = {
		knowledge_base: Database,
		chat: MessageCircle,
		search: Search,
		app_dev: Bot
	};

	// Fades/rises each section into place as it enters the viewport. Runs once
	// per element; a no-op under prefers-reduced-motion via the CSS below.
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
			{ threshold: 0.2, rootMargin: '0px 0px -10% 0px' }
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

<!-- Hero banner -->
<section class="mx-auto max-w-6xl px-6 py-24 md:py-32 lg:py-40">
	<div use:reveal class="reveal max-w-3xl">
		<h1
			class="text-[clamp(2.5rem,4.5vw+1rem,4.25rem)] leading-[1.05] font-semibold tracking-tight text-foreground"
		>
			{cfg.hero.slogan}
		</h1>
		<p class="mt-6 max-w-[60ch] text-lg text-muted-foreground">{cfg.hero.subtitle}</p>
		<div class="mt-10 flex flex-wrap items-center gap-4">
			<a
				href={cfg.hero.cta_primary_href}
				class="group inline-flex items-center gap-2 rounded-md bg-primary px-5 py-2.5 text-sm font-medium text-primary-foreground transition-[opacity,transform] duration-200 hover:opacity-90 active:scale-[0.98] focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
			>
				{m.semos_get_started()}
				<ArrowRight class="h-4 w-4 transition-transform duration-200 group-hover:translate-x-0.5" />
			</a>
			<a
				href={cfg.hero.cta_secondary_href}
				class="inline-flex items-center rounded-md border border-border px-5 py-2.5 text-sm font-medium text-foreground transition-colors duration-200 hover:border-foreground/20 hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
			>
				{m.semos_signup_login()}
			</a>
		</div>
	</div>
</section>

<!-- Product highlights (~5, alternating) -->
<section class="border-t border-border/70">
	<div class="mx-auto max-w-6xl space-y-24 px-6 py-24 md:space-y-32 md:py-32">
		{#each cfg.highlights as h, i (h.title)}
			<div
				use:reveal
				class="reveal grid items-center gap-10 md:grid-cols-2 md:gap-16 {i % 2 === 1
					? 'md:[&>*:first-child]:order-2'
					: ''}"
			>
				<div>
					<span class="text-sm font-medium tabular-nums text-muted-foreground/70"
						>{String(i + 1).padStart(2, '0')}</span
					>
					<h2 class="mt-3 text-2xl font-semibold tracking-tight text-foreground md:text-3xl">
						{h.title}
					</h2>
					<p class="mt-3 max-w-[50ch] text-muted-foreground">{h.description}</p>
				</div>
				<div class="overflow-hidden rounded-lg border border-border">
					<img
						src={h.image}
						alt={h.title}
						class="aspect-video w-full object-cover transition-transform duration-500 ease-out hover:scale-105"
					/>
				</div>
			</div>
		{/each}
	</div>
</section>

<!-- Main features (4 cards) -->
<section class="border-t border-border/70">
	<div use:reveal class="reveal mx-auto max-w-6xl px-6 py-24 md:py-32">
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
			{#each cfg.features as f, i (f.key)}
				{@const Icon = featureIcons[f.key] ?? Sparkles}
				<a
					href={f.href}
					style="transition-delay: {i * 60}ms"
					class="group rounded-lg border border-border p-6 transition-colors duration-200 hover:border-foreground/20 hover:bg-accent/60 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
				>
					<div class="flex items-start justify-between">
						<Icon class="h-5 w-5 text-foreground" />
						<ArrowUpRight
							class="h-4 w-4 text-muted-foreground/60 transition-transform duration-200 group-hover:translate-x-0.5 group-hover:-translate-y-0.5"
						/>
					</div>
					<h3 class="mt-4 font-semibold tracking-tight text-foreground">{f.title}</h3>
					<p class="mt-2 text-sm text-muted-foreground">{f.description}</p>
				</a>
			{/each}
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

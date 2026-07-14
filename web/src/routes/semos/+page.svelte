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

<!-- ════════════════════════════════════════════
     Hero — gradient mesh bg, grid dots, larger type
     ════════════════════════════════════════════ -->
<section class="relative mx-auto max-w-6xl px-6 py-28 md:py-40 lg:py-48">
	<!-- Decorative background: grid dots + gradient flare -->
	<div
		class="pointer-events-none absolute inset-0 -z-10 overflow-hidden [mask-image:radial-gradient(ellipse_60%_70%_at_50%_20%,black,transparent_70%)]"
	>
		<div class="absolute inset-0 bg-[image:radial-gradient(var(--muted-foreground)/.12_1px,transparent_0)] bg-[size:24px_24px]"></div>
		<div
			class="absolute left-1/2 top-0 h-[40rem] w-[40rem] -translate-x-1/2 rounded-full opacity-[.07] blur-[100px]"
			style="background: radial-gradient(circle, var(--primary) 0%, transparent 70%)"
		></div>
	</div>

	<div use:reveal class="reveal max-w-3xl">
		<h1
			class="text-[clamp(2.75rem,5vw+1rem,5rem)] leading-[1.04] font-semibold tracking-tight text-foreground"
		>
			{cfg.hero.slogan}
		</h1>
		<p class="mt-6 max-w-[58ch] text-base leading-relaxed text-muted-foreground md:text-lg">
			{cfg.hero.subtitle}
		</p>
		<div class="mt-10 flex flex-wrap items-center gap-4">
			<a
				href={cfg.hero.cta_primary_href}
				class="group inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground shadow-sm transition-all duration-200 hover:opacity-90 hover:shadow-md active:scale-[0.97] focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
			>
				{m.semos_get_started()}
				<ArrowRight
					class="h-4 w-4 transition-transform duration-200 group-hover:translate-x-0.5"
				/>
			</a>
			<a
				href={cfg.hero.cta_secondary_href}
				class="inline-flex items-center rounded-lg border border-border px-6 py-3 text-sm font-medium text-foreground shadow-sm transition-all duration-200 hover:border-foreground/20 hover:bg-accent hover:shadow-sm active:scale-[0.97] focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
			>
				{m.semos_signup_login()}
			</a>
		</div>
	</div>

	<!-- Decorative floating dots (large viewport only) -->
	<div
		class="pointer-events-none absolute -right-8 top-1/3 hidden h-12 w-12 rounded-full border border-border/40 bg-accent/30 lg:block"
	></div>
	<div
		class="pointer-events-none absolute -left-6 bottom-1/4 hidden h-8 w-8 rounded-full border border-border/30 bg-accent/20 lg:block"
	></div>
</section>

<!-- ════════════════════════════════════════════
     Product highlights — each a framed card with a
     stepped progression line
     ════════════════════════════════════════════ -->
<section class="border-t border-border/70">
	<div class="relative mx-auto max-w-6xl px-6 py-24 md:py-32">
		<!-- Vertical progression line (desktop) -->
		<div
			class="absolute left-6 top-0 bottom-0 hidden w-px bg-gradient-to-b from-transparent via-border/40 to-transparent md:left-1/2 md:-translate-x-px md:block"
		></div>

		<div class="space-y-28 md:space-y-36">
			{#each cfg.highlights as h, i (h.title)}
				<div
					use:reveal
					class="reveal relative grid gap-8 md:grid-cols-2 md:gap-16 {i % 2 === 1
						? 'md:[&>*:first-child]:order-2'
						: ''}"
				>
					<!-- Text side -->
					<div class="flex flex-col justify-center">
						<div class="flex items-center gap-3">
							<span
								class="flex h-9 w-9 items-center justify-center rounded-full border border-border/60 bg-background text-[13px] font-semibold tabular-nums text-foreground shadow-sm"
							>
								{i + 1}
							</span>
							<span
								class="h-px flex-1 bg-gradient-to-r from-border/60 to-border/20 md:{i % 2 === 1
									? '[mask-image:linear-gradient(to_left,black,transparent_80%)]'
									: '[mask-image:linear-gradient(to_right,black,transparent_80%)]'}"
							></span>
						</div>
						<h2
							class="mt-5 text-2xl font-semibold tracking-tight text-foreground md:text-3xl md:leading-snug"
						>
							{h.title}
						</h2>
						<p class="mt-3 max-w-[48ch] leading-relaxed text-muted-foreground">
							{h.description}
						</p>
					</div>

					<!-- Image side -->
					<div class="overflow-hidden rounded-xl border border-border bg-accent/30 shadow-sm">
						<img
							src={h.image}
							alt={h.title}
							class="aspect-video w-full object-cover transition-all duration-500 ease-out hover:scale-105"
						/>
					</div>
				</div>
			{/each}
		</div>
	</div>
</section>

<!-- ════════════════════════════════════════════
     Features — elevated card with icon well
     ════════════════════════════════════════════ -->
<section class="border-t border-border/70 bg-accent/20">
	<div use:reveal class="reveal mx-auto max-w-6xl px-6 py-24 md:py-32">
		<h2 class="text-center text-2xl font-semibold tracking-tight text-foreground md:text-3xl">
			{cfg.features_section.title}
		</h2>
		<p class="mt-3 text-center text-muted-foreground">
			{cfg.features_section.subtitle}
		</p>
		<div class="mt-12 grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
			{#each cfg.features as f, i (f.key)}
				{@const Icon = featureIcons[f.key] ?? Sparkles}
				<a
					href={f.href}
					style="transition-delay: {i * 80}ms"
					class="group flex flex-col rounded-xl border border-border/70 bg-background p-6 shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:border-foreground/15 hover:shadow-md focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
				>
					<div class="inline-flex w-fit rounded-lg bg-primary/10 p-3 text-primary">
						<Icon class="h-5 w-5" />
					</div>
					<div class="mt-5 flex items-start justify-between">
						<h3 class="font-semibold tracking-tight text-foreground">{f.title}</h3>
						<ArrowUpRight
							class="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground/50 transition-all duration-200 group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:text-foreground"
						/>
					</div>
					<p class="mt-2 text-sm leading-relaxed text-muted-foreground">{f.description}</p>
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

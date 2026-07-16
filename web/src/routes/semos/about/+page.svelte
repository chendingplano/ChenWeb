<script lang="ts">
	import { ArrowUpRight } from '@lucide/svelte';
	import Ornament from '../components/Ornament.svelte';

	let { data } = $props();
	const cfg = $derived(data.siteConfig);
	const about = $derived(cfg.about);

	// Stacked headline, same treatment as the home hero, but split on both
	// ASCII and CJK commas since this page's copy is Chinese-first.
	const titleLines = $derived(
		about.title
			.split(/([,，]\s*)/)
			.reduce<string[]>((lines, part, i) => {
				if (i % 2 === 0) lines.push(part);
				else lines[lines.length - 1] += part.trim();
				return lines;
			}, [])
			.filter(Boolean)
	);

	const story = $derived(about.story ?? []);
	const values = $derived(about.values ?? []);

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
	<title>{about.kicker} · {cfg.branding.site_name}</title>
	<meta name="description" content={about.lede} />
</svelte:head>

<!-- ═════════════════════════════════════════════════
     HERO — no photograph here; the About page opens
     like a book's frontispiece: type on paper, with
     the bronze diamond blown up to architectural
     scale as a watermark.
     ═════════════════════════════════════════════════ -->
<section class="relative overflow-hidden">
	<!-- Watermark: the site ornament's diamond, enormous and almost silent -->
	<div
		aria-hidden="true"
		class="pointer-events-none absolute top-1/2 right-[-6rem] hidden -translate-y-1/2 lg:block"
	>
		<div class="h-[26rem] w-[26rem] rotate-45 border border-[#b08d57]/20"></div>
		<div
			class="absolute top-1/2 left-1/2 h-[17rem] w-[17rem] -translate-x-1/2 -translate-y-1/2 rotate-45 border border-[#b08d57]/12"
		></div>
		<div
			class="absolute top-1/2 left-1/2 h-4 w-4 -translate-x-1/2 -translate-y-1/2 rotate-45 bg-[#b08d57]/60"
		></div>
	</div>

	<div class="relative mx-auto max-w-7xl px-6 pt-24 pb-20 md:pt-32 md:pb-28">
		<div use:reveal class="reveal max-w-3xl">
			<div class="flex items-center gap-3">
				<span class="inline-block h-1.5 w-1.5 rotate-45 bg-[#b08d57]"></span>
				<span
					class="text-xs font-bold tracking-[0.22em] text-[#6f6c66] uppercase dark:text-[#a5a29b]"
				>
					{about.kicker}
				</span>
			</div>

			<h1
				class="mt-7 text-[clamp(2.4rem,4vw+1rem,4rem)] leading-[1.14] font-bold tracking-tight text-[#17181c] dark:text-[#e9e7e2]"
			>
				{#each titleLines as line (line)}
					<span class="block">{line}</span>
				{/each}
			</h1>

			<p
				class="mt-8 max-w-[42ch] text-lg leading-[1.9] text-[#6f6c66] md:text-xl dark:text-[#a5a29b]"
			>
				{about.lede}
			</p>
		</div>
	</div>
</section>

<!-- ═════════════════════════════════════════════════
     STORY — an editorial spread: kicker in a narrow
     margin column (sticky on tall screens), prose in
     a measured column beside it, the photograph
     hanging below in the site's floating frame.
     ═════════════════════════════════════════════════ -->
{#if story.length > 0}
	<section class="relative">
		<div class="mx-auto max-w-7xl px-6 pb-24 md:pb-32">
			<div class="grid gap-10 md:grid-cols-[200px_1fr] md:gap-16 lg:grid-cols-[240px_1fr]">
				<div>
					<div class="md:sticky md:top-28">
						<div class="flex items-center gap-3">
							<span class="inline-block h-1.5 w-1.5 rotate-45 bg-[#b08d57]"></span>
							<span class="text-xs font-bold tracking-[0.22em] text-[#b08d57] uppercase">
								{about.story_kicker}
							</span>
						</div>
						<div class="mt-6 hidden h-24 w-px bg-[#17181c]/10 md:block dark:bg-white/10"></div>
					</div>
				</div>

				<div class="max-w-[65ch]">
					{#each story as paragraph, i (paragraph)}
						<p
							use:reveal
							class="reveal leading-[2] text-[#3d3c38] [&:not(:first-child)]:mt-7 dark:text-[#c6c3bc]"
							style="transition-delay: {i * 80}ms"
						>
							{paragraph}
						</p>
					{/each}
				</div>
			</div>

			{#if about.image}
				<div use:reveal class="reveal mt-16 md:mt-24 md:px-16 lg:px-28">
					<div class="group relative">
						<div
							class="absolute inset-x-8 -bottom-4 h-8 rounded-[50%] bg-[#17181c]/10 blur-xl transition-all duration-500 group-hover:inset-x-6 group-hover:bg-[#17181c]/14 dark:bg-black/40"
						></div>
						<div
							class="relative overflow-hidden rounded-xl border-t border-white bg-white p-2.5 shadow-[0_1px_2px_rgba(23,24,28,0.08),0_12px_32px_rgba(23,24,28,0.14)] transition-all duration-500 group-hover:shadow-[0_2px_4px_rgba(23,24,28,0.1),0_20px_48px_rgba(23,24,28,0.18)] dark:border-white/10 dark:bg-[#1a1d24]"
						>
							<img
								src={about.image}
								alt={about.story_kicker}
								class="aspect-video w-full rounded-lg object-cover md:aspect-[21/9]"
							/>
						</div>
					</div>
				</div>
			{/if}
		</div>
	</section>
{/if}

<!-- ═════════════════════════════════════════════════
     VALUES — the page's one drenched moment: paper
     inverts to ink, and the convictions read as a
     numbered manifesto, ruled like a ledger. No
     cards; hairlines and bronze numerals carry it.
     ═════════════════════════════════════════════════ -->
{#if values.length > 0}
	<section class="relative bg-[#17181c] dark:bg-[#0b0d10]">
		<div class="mx-auto max-w-7xl px-6 py-24 md:py-32">
			<div use:reveal class="reveal">
				<div class="flex items-center gap-3">
					<span class="inline-block h-1.5 w-1.5 rotate-45 bg-[#b08d57]"></span>
					<span class="text-xs font-bold tracking-[0.22em] text-[#b08d57] uppercase">
						{about.values_kicker}
					</span>
				</div>
				<h2
					class="mt-5 text-3xl font-bold tracking-tight text-[#f5f3ef] md:text-4xl"
				>
					{about.values_title}
				</h2>
			</div>

			<div class="mt-14 border-t border-white/12 md:mt-16">
				{#each values as v, i (v.title)}
					<div
						use:reveal
						class="reveal grid gap-4 border-b border-white/12 py-10 md:grid-cols-[7rem_minmax(14rem,18rem)_1fr] md:gap-10 md:py-12"
						style="transition-delay: {i * 90}ms"
					>
						<span
							class="text-[2.75rem] leading-none font-bold tracking-tight text-[#b08d57] tabular-nums md:text-[3.25rem]"
						>
							{String(i + 1).padStart(2, '0')}
						</span>
						<h3 class="text-xl leading-snug font-bold tracking-tight text-[#f5f3ef] md:text-2xl">
							{v.title}
						</h3>
						<p class="max-w-[52ch] leading-[1.95] text-[#a5a29b]">
							{v.description}
						</p>
					</div>
				{/each}
			</div>
		</div>
	</section>
{/if}

<!-- ═════════════════════════════════════════════════
     CONTACT — back on paper, quiet. The email is the
     hero here: one large bronze-underlined line, with
     the address kept small beside it.
     ═════════════════════════════════════════════════ -->
<section class="relative">
	<div class="mx-auto max-w-7xl px-6 py-24 md:py-32">
		<div use:reveal class="reveal">
			<Ornament class="pb-14 md:pb-16" />

			<div class="grid gap-10 md:grid-cols-2 md:gap-16">
				<div>
					<div class="flex items-center gap-3">
						<span class="inline-block h-1.5 w-1.5 rotate-45 bg-[#b08d57]"></span>
						<span class="text-xs font-bold tracking-[0.22em] text-[#b08d57] uppercase">
							{about.contact_kicker}
						</span>
					</div>
					<h2
						class="mt-5 text-3xl font-bold tracking-tight text-[#17181c] md:text-4xl dark:text-[#e9e7e2]"
					>
						{about.contact_title}
					</h2>
					<p class="mt-5 max-w-[42ch] leading-[1.9] text-[#6f6c66] dark:text-[#a5a29b]">
						{about.contact_text}
					</p>
				</div>

				<div class="md:pt-1 md:text-right">
					<a
						href="mailto:{cfg.footer.email}"
						class="group inline-flex items-baseline gap-2 text-2xl font-bold tracking-tight break-all text-[#17181c] underline decoration-[#b08d57]/40 decoration-2 underline-offset-8 transition-colors duration-200 hover:decoration-[#b08d57] focus-visible:ring-2 focus-visible:ring-[#b08d57]/50 focus-visible:outline-none md:text-3xl dark:text-[#e9e7e2]"
					>
						{cfg.footer.email}
						<ArrowUpRight
							class="h-5 w-5 shrink-0 self-center text-[#b08d57] transition-transform duration-200 group-hover:translate-x-0.5 group-hover:-translate-y-0.5"
						/>
					</a>
					{#if cfg.footer.address}
						<p
							class="mt-8 text-sm leading-relaxed whitespace-pre-line text-[#6f6c66] dark:text-[#a5a29b]"
						>
							{cfg.footer.address}
						</p>
					{/if}
				</div>
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

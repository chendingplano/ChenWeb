<script lang="ts">
	import type { SiteConfig } from '$lib/services/siteConfigService';
	import { semosTheme } from '$lib/stores/semosTheme.svelte';
	import { locales, getLocale, setLocale } from '$lib/paraglide/runtime';
	import { Sun, Moon, Languages, Menu, X } from '@lucide/svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { slide } from 'svelte/transition';
	import LogoMark from './LogoMark.svelte';

	let { config }: { config: SiteConfig } = $props();

	const nav = [
		{ label: m.semos_nav_home(), href: '/semos' },
		{ label: m.semos_nav_workspace(), href: '/semos/workspace' },
		{ label: m.semos_nav_knowledge_base(), href: '/home3/knowledge' },
		{ label: m.semos_nav_about(), href: '/semos' }
	];

	let mobileOpen = $state(false);

	function nextLocale(): (typeof locales)[number] {
		const idx = locales.indexOf(getLocale());
		return locales[(idx + 1) % locales.length];
	}
</script>

<header
	class="sticky top-0 z-50 w-full border-b border-[#17181c]/8 bg-[#faf9f7]/90 backdrop-blur-xl dark:border-white/10 dark:bg-[#101216]/90"
>
	<div class="mx-auto flex h-[4.25rem] max-w-7xl items-center justify-between px-6">
		<!-- Wordmark: image logo if configured, else bronze-tick monogram + text -->
		<a href="/semos" class="group flex items-baseline gap-1.5">
			<LogoMark
				branding={config.branding}
				textClass="text-[1.1rem] font-bold tracking-[0.02em] text-[#17181c] dark:text-[#e9e7e2]"
			/>
		</a>

		<nav class="hidden items-center gap-7 md:flex">
			{#each nav as item (item.href + item.label)}
				<a
					href={item.href}
					class="text-[0.9rem] font-medium text-[#17181c]/60 transition-colors duration-200 hover:text-[#17181c] dark:text-white/60 dark:hover:text-white"
				>
					{item.label}
				</a>
			{/each}
		</nav>

		<div class="flex items-center gap-1.5">
			<button
				type="button"
				class="rounded-full p-2 text-[#17181c]/45 transition-colors duration-200 hover:bg-[#17181c]/5 hover:text-[#17181c] dark:text-white/45 dark:hover:bg-white/10 dark:hover:text-white"
				aria-label="Switch language"
				onclick={() => setLocale(nextLocale())}
			>
				<Languages class="h-4 w-4" />
			</button>
			<button
				type="button"
				class="rounded-full p-2 text-[#17181c]/45 transition-colors duration-200 hover:bg-[#17181c]/5 hover:text-[#17181c] dark:text-white/45 dark:hover:bg-white/10 dark:hover:text-white"
				aria-label="Toggle dark mode"
				onclick={() => semosTheme.toggle()}
			>
				{#if semosTheme.mode === 'dark'}
					<Sun class="h-4 w-4" />
				{:else}
					<Moon class="h-4 w-4" />
				{/if}
			</button>
			<a
				href={config.hero.cta_secondary_href}
				class="ml-2 hidden rounded-lg bg-[#17181c] px-5 py-2.5 text-sm font-semibold text-white shadow-[0_1px_2px_rgba(23,24,28,0.2),0_6px_16px_rgba(23,24,28,0.18)] transition-all duration-200 hover:-translate-y-px hover:shadow-[0_2px_4px_rgba(23,24,28,0.2),0_10px_24px_rgba(23,24,28,0.22)] active:translate-y-0 sm:inline-flex dark:bg-white dark:text-[#17181c]"
			>
				{m.semos_signup_login()}
			</a>
			<button
				type="button"
				class="rounded-full p-2 text-[#17181c]/45 transition-colors hover:bg-[#17181c]/5 hover:text-[#17181c] md:hidden dark:text-white/45 dark:hover:bg-white/10 dark:hover:text-white"
				aria-label={mobileOpen ? 'Close menu' : 'Open menu'}
				aria-expanded={mobileOpen}
				aria-controls="semos-mobile-nav"
				onclick={() => (mobileOpen = !mobileOpen)}
			>
				{#if mobileOpen}
					<X class="h-5 w-5" />
				{:else}
					<Menu class="h-5 w-5" />
				{/if}
			</button>
		</div>
	</div>

	{#if mobileOpen}
		<nav
			id="semos-mobile-nav"
			transition:slide={{ duration: 200 }}
			class="border-t border-[#17181c]/8 bg-[#faf9f7] md:hidden dark:border-white/10 dark:bg-[#101216]"
		>
			<div class="mx-auto flex max-w-7xl flex-col px-6 py-2">
				{#each nav as item (item.href + item.label)}
					<a
						href={item.href}
						class="rounded-md px-2 py-2.5 text-sm font-medium text-[#17181c]/60 transition-colors hover:text-[#17181c] dark:text-white/60 dark:hover:text-white"
						onclick={() => (mobileOpen = false)}
					>
						{item.label}
					</a>
				{/each}
				<a
					href={config.hero.cta_secondary_href}
					class="mt-2 inline-flex items-center justify-center rounded-lg bg-[#17181c] px-4 py-2.5 text-sm font-semibold text-white sm:hidden dark:bg-white dark:text-[#17181c]"
					onclick={() => (mobileOpen = false)}
				>
					{m.semos_signup_login()}
				</a>
			</div>
		</nav>
	{/if}
</header>

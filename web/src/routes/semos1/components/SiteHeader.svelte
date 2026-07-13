<script lang="ts">
	import type { SiteConfig } from '$lib/services/siteConfigService';
	import { semosTheme } from '$lib/stores/semosTheme.svelte';
	import { locales, getLocale, setLocale } from '$lib/paraglide/runtime';
	import { Sun, Moon, Languages, Menu, X } from '@lucide/svelte';
	import { m } from '$lib/paraglide/messages.js';

	let { config }: { config: SiteConfig } = $props();

	const nav = [
		{ label: m.semos_nav_home(), href: '/semos1' },
		{ label: m.semos_nav_workspace(), href: '/semos1/workspace' },
		{ label: m.semos_nav_knowledge_base(), href: '/home3/knowledge' }
	];

	let mobileOpen = $state(false);

	function nextLocale(): (typeof locales)[number] {
		const idx = locales.indexOf(getLocale());
		return locales[(idx + 1) % locales.length];
	}
</script>

<header
	class="sticky top-0 z-50 w-full border-b border-white/10 bg-[#0c0f1a]/95 backdrop-blur-xl"
>
	<div class="mx-auto flex h-16 max-w-7xl items-center justify-between px-6">
		<a
			href="/semos1"
			class="text-base font-bold tracking-tight text-white transition-opacity hover:opacity-80"
		>
			{config.branding.logo_text}
		</a>

		<nav class="hidden items-center gap-1 md:flex">
			{#each nav as item (item.href + item.label)}
				<a
					href={item.href}
					class="rounded-md px-3 py-2 text-sm font-medium text-white/60 transition-colors duration-200 hover:bg-white/10 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/30"
				>
					{item.label}
				</a>
			{/each}
		</nav>

		<div class="flex items-center gap-1">
			<button
				type="button"
				class="rounded-md p-2 text-white/50 transition-colors duration-200 hover:bg-white/10 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/30"
				aria-label="Switch language"
				onclick={() => setLocale(nextLocale())}
			>
				<Languages class="h-4 w-4" />
			</button>
			<button
				type="button"
				class="rounded-md p-2 text-white/50 transition-colors duration-200 hover:bg-white/10 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/30"
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
				class="ml-2 hidden items-center gap-1.5 rounded-lg bg-white px-4 py-2 text-sm font-semibold text-[#0c0f1a] transition-all duration-200 hover:bg-white/90 active:scale-[0.97] sm:inline-flex"
			>
				{m.semos_signup_login()}
			</a>
			<button
				type="button"
				class="ml-1 rounded-md p-2 text-white/50 transition-colors duration-200 hover:bg-white/10 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/30 md:hidden"
				aria-label={mobileOpen ? 'Close menu' : 'Open menu'}
				aria-expanded={mobileOpen}
				aria-controls="semos1-mobile-nav"
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
			id="semos1-mobile-nav"
			class="border-t border-white/10 bg-[#0c0f1a] md:hidden"
		>
			<div class="mx-auto flex max-w-7xl flex-col px-6 py-2">
				{#each nav as item (item.href + item.label)}
					<a
						href={item.href}
						class="rounded-md px-2 py-2.5 text-sm font-medium text-white/60 transition-colors hover:bg-white/10 hover:text-white"
						onclick={() => (mobileOpen = false)}
					>
						{item.label}
					</a>
				{/each}
				<a
					href={config.hero.cta_secondary_href}
					class="mt-2 inline-flex items-center justify-center rounded-lg bg-white px-4 py-2.5 text-sm font-semibold text-[#0c0f1a] sm:hidden"
					onclick={() => (mobileOpen = false)}
				>
					{m.semos_signup_login()}
				</a>
			</div>
		</nav>
	{/if}
</header>

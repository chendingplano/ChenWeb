<script lang="ts">
	import type { SiteConfig } from '$lib/services/siteConfigService';
	import { semosTheme } from '$lib/stores/semosTheme.svelte';
	import { locales, getLocale, setLocale } from '$lib/paraglide/runtime';
	import { Sun, Moon, Languages, Menu, X } from '@lucide/svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { slide } from 'svelte/transition';

	let { config }: { config: SiteConfig } = $props();

	// Recomputed on every mount. The default paraglide strategy reloads the
	// page on setLocale, so this always reflects the active locale — no extra
	// reactivity plumbing needed (matches src/routes/demo/paraglide/+page.svelte).
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
	class="sticky top-0 z-40 w-full border-b border-border/70 bg-background/80 backdrop-blur-md"
>
	<div class="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
		<a
			href="/semos"
			class="text-[1.05rem] font-semibold tracking-tight text-foreground transition-opacity hover:opacity-80"
		>
			{config.branding.logo_text}
		</a>

		<nav class="hidden items-center gap-1 md:flex">
			{#each nav as item (item.href + item.label)}
				<a
					href={item.href}
					class="rounded-md px-3 py-2 text-sm font-medium text-muted-foreground transition-colors duration-200 hover:bg-accent/60 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
				>
					{item.label}
				</a>
			{/each}
		</nav>

		<div class="flex items-center gap-1.5">
			<button
				type="button"
				class="rounded-md p-2 text-muted-foreground transition-colors duration-200 hover:bg-accent/60 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
				aria-label="Switch language"
				onclick={() => setLocale(nextLocale())}
			>
				<Languages class="h-4 w-4" />
			</button>
			<button
				type="button"
				class="rounded-md p-2 text-muted-foreground transition-colors duration-200 hover:bg-accent/60 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
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
				class="ml-1 hidden items-center gap-1.5 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-[opacity,transform] duration-200 hover:opacity-90 active:scale-[0.98] focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none sm:inline-flex"
			>
				{m.semos_signup_login()}
			</a>
			<button
				type="button"
				class="rounded-md p-2 text-muted-foreground transition-colors duration-200 hover:bg-accent/60 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none md:hidden"
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
			class="border-t border-border/70 bg-background md:hidden"
		>
			<div class="mx-auto flex max-w-6xl flex-col px-6 py-2">
				{#each nav as item (item.href + item.label)}
					<a
						href={item.href}
						class="rounded-md px-2 py-2.5 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent/60 hover:text-foreground"
						onclick={() => (mobileOpen = false)}
					>
						{item.label}
					</a>
				{/each}
				<a
					href={config.hero.cta_secondary_href}
					class="mt-2 inline-flex items-center justify-center rounded-md bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90 sm:hidden"
					onclick={() => (mobileOpen = false)}
				>
					{m.semos_signup_login()}
				</a>
			</div>
		</nav>
	{/if}
</header>

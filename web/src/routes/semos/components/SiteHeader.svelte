<script lang="ts">
	import type { SiteConfig } from '$lib/services/siteConfigService';
	import { theme } from '$lib/stores/theme.svelte';
	import { locales, getLocale, setLocale } from '$lib/paraglide/runtime';
	import { Sun, Moon, Languages, Menu, X } from '@lucide/svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { slide } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { appAuthStore } from '@chendingplano/shared';
	import LogoMark from './LogoMark.svelte';
	import { loginPrompt } from '../loginPrompt.svelte';

	let { config }: { config: SiteConfig } = $props();

	const nav = [
		{ label: m.semos_nav_home(), href: '/semos' },
		{ label: m.semos_nav_workspace(), href: '/semos/workspace', requiresAuth: true },
		{ label: m.semos_nav_knowledge_base(), href: '/home3/knowledge', requiresAuth: true },
		{ label: m.semos_nav_about(), href: '/semos' }
	];

	let mobileOpen = $state(false);

	async function handleNavClick(event: MouseEvent, item: (typeof nav)[number]) {
		if (item.requiresAuth && !appAuthStore.getIsLoggedIn()) {
			event.preventDefault();
			if (page.url.pathname !== '/semos') {
				await goto('/semos');
			}
			window.scrollTo({ top: 0, behavior: 'smooth' });
			loginPrompt.show();
		}
	}

	function isActive(item: (typeof nav)[number]): boolean {
		// "关于我们" aliases the same href as "首页" (no dedicated About page), so
		// match by first-in-array-order rather than href alone — otherwise both
		// would highlight together whenever the href is the current path.
		return nav.find((n) => n.href === page.url.pathname) === item;
	}

	function nextLocale(): (typeof locales)[number] {
		const idx = locales.indexOf(getLocale());
		return locales[(idx + 1) % locales.length];
	}

	async function handleLogout() {
		// Not appAuthStore.logout() — that does a hard window.location redirect
		// to '/login' (the backend's default redirect_url), which always wins
		// over a subsequent goto('/semos'). The backend already tears down the
		// Kratos session and app cookie synchronously, so it's safe to do our
		// own POST + resync here and navigate with SvelteKit's router instead.
		await fetch('/auth/logout', { method: 'POST', credentials: 'include' });
		await appAuthStore.checkAuthStatus();
		await goto('/semos');
	}
</script>

<header
	class="sticky top-0 z-50 w-full border-b border-[#17181c]/8 bg-[#faf9f7]/90 backdrop-blur-xl dark:border-white/10 dark:bg-[#101216]/90"
>
	<div class="mx-auto flex h-[4.25rem] max-w-7xl items-center justify-between px-6">
		<!-- Wordmark: image logo if configured, else bronze-tick monogram + text -->
		<a href="/semos" class="flex items-baseline gap-1.5">
			<LogoMark
				branding={config.branding}
				textClass="text-[1.1rem] font-bold tracking-[0.02em] text-[#17181c] dark:text-[#e9e7e2]"
			/>
		</a>

		<nav class="hidden items-center gap-1 md:flex">
			{#each nav as item (item.href + item.label)}
				<a
					href={item.href}
					onclick={(e) => handleNavClick(e, item)}
					class="rounded-md px-3 py-1.5 text-[0.9rem] font-medium transition-colors duration-200 {isActive(
						item
					)
						? 'bg-[#17181c]/8 text-[#17181c] dark:bg-white/10 dark:text-white'
						: 'text-[#17181c]/60 hover:text-[#17181c] dark:text-white/60 dark:hover:text-white'}"
				>
					{item.label}
				</a>
			{/each}
		</nav>

		<div class="flex items-center gap-1.5">
			<button
				type="button"
				class="flex items-center gap-1.5 rounded-full px-2.5 py-2 text-[#17181c]/45 transition-colors duration-200 hover:bg-[#17181c]/5 hover:text-[#17181c] dark:text-white/45 dark:hover:bg-white/10 dark:hover:text-white"
				aria-label="Switch language"
				onclick={() => setLocale(nextLocale())}
			>
				<Languages class="h-4 w-4" />
				<span class="text-[0.8rem] font-medium">{getLocale() === 'zh-cn' ? '中文' : 'English'}</span
				>
			</button>
			<button
				type="button"
				class="rounded-full p-2 text-[#17181c]/45 transition-colors duration-200 hover:bg-[#17181c]/5 hover:text-[#17181c] dark:text-white/45 dark:hover:bg-white/10 dark:hover:text-white"
				aria-label="Toggle dark mode"
				onclick={() => theme.toggle()}
			>
				{#if theme.isDark}
					<Sun class="h-4 w-4" />
				{:else}
					<Moon class="h-4 w-4" />
				{/if}
			</button>
			{#if $appAuthStore.isLoggedIn}
				<button
					type="button"
					onclick={handleLogout}
					class="ml-2 hidden rounded-lg bg-[#17181c] px-5 py-2.5 text-sm font-semibold text-white shadow-[0_1px_2px_rgba(23,24,28,0.2),0_6px_16px_rgba(23,24,28,0.18)] transition-all duration-200 hover:-translate-y-px hover:shadow-[0_2px_4px_rgba(23,24,28,0.2),0_10px_24px_rgba(23,24,28,0.22)] active:translate-y-0 sm:inline-flex dark:bg-white dark:text-[#17181c]"
				>
					{m.semos_logout()}
				</button>
			{/if}
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
						class="rounded-md px-2 py-2.5 text-sm font-medium transition-colors {isActive(item)
							? 'bg-[#17181c]/8 text-[#17181c] dark:bg-white/10 dark:text-white'
							: 'text-[#17181c]/60 hover:text-[#17181c] dark:text-white/60 dark:hover:text-white'}"
						onclick={(e) => {
							handleNavClick(e, item);
							mobileOpen = false;
						}}
					>
						{item.label}
					</a>
				{/each}
				{#if $appAuthStore.isLoggedIn}
					<button
						type="button"
						class="mt-2 inline-flex items-center justify-center rounded-lg bg-[#17181c] px-4 py-2.5 text-sm font-semibold text-white sm:hidden dark:bg-white dark:text-[#17181c]"
						onclick={() => {
							mobileOpen = false;
							handleLogout();
						}}
					>
						{m.semos_logout()}
					</button>
				{/if}
			</div>
		</nav>
	{/if}
</header>

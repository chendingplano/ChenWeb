<script lang="ts">
	import type { SiteConfig } from '$lib/services/siteConfigService';
	import { m } from '$lib/paraglide/messages.js';
	import { ArrowUpRight } from '@lucide/svelte';

	let { config }: { config: SiteConfig } = $props();

	// Nav links stay i18n-driven (they mirror the header); the rest of the
	// footer's link columns and contact details come from site config.
	const quickLinks = $derived([
		{ label: m.semos_nav_home(), href: '/semos1' },
		{ label: m.semos_nav_workspace(), href: '/semos1/workspace' },
		...config.footer.quick_links
	]);

	const resources = $derived(config.footer.resources);
</script>

<footer class="relative bg-[#0c0f1a]">
	<!-- Subtle top border glow -->
	<div class="absolute top-0 left-0 right-0 h-px bg-gradient-to-r from-transparent via-[#6b7aff]/30 to-transparent"></div>

	<div class="mx-auto max-w-7xl px-6 py-16 md:py-20">
		<div class="grid gap-10 sm:grid-cols-2 lg:grid-cols-4">
			<!-- Column 1: Company info -->
			<div>
				<div class="text-base font-bold tracking-tight text-white">
					{config.branding.logo_text}
				</div>
				<p class="mt-4 text-sm leading-relaxed text-white/45">
					{config.footer.text}
				</p>
				{#if config.footer.address}
					<p class="mt-4 text-sm whitespace-pre-line text-white/45">{config.footer.address}</p>
				{/if}
			</div>

			<!-- Column 2: Quick Links -->
			<div>
				<h3 class="text-xs font-bold tracking-wider text-white/50 uppercase">
					Quick Links
				</h3>
				<ul class="mt-6 space-y-3">
					{#each quickLinks as link (link.label)}
						<li>
							<a
								href={link.href}
								class="group inline-flex items-center gap-1.5 text-sm text-white/70 transition-colors duration-200 hover:text-white"
							>
								{link.label}
								<ArrowUpRight class="h-3 w-3 opacity-0 transition-all duration-200 group-hover:opacity-60" />
							</a>
						</li>
					{/each}
				</ul>
			</div>

			<!-- Column 3: Resources -->
			<div>
				<h3 class="text-xs font-bold tracking-wider text-white/50 uppercase">
					Resources
				</h3>
				<ul class="mt-6 space-y-3">
					{#each resources as link (link.label)}
						<li>
							<a
								href={link.href}
								class="group inline-flex items-center gap-1.5 text-sm text-white/70 transition-colors duration-200 hover:text-white"
							>
								{link.label}
								<ArrowUpRight class="h-3 w-3 opacity-0 transition-all duration-200 group-hover:opacity-60" />
							</a>
						</li>
					{/each}
				</ul>
			</div>

			<!-- Column 4: Contact / Social -->
			<div>
				<h3 class="text-xs font-bold tracking-wider text-white/50 uppercase">
					Stay Connected
				</h3>
				<p class="mt-6 text-sm leading-relaxed text-white/45">
					{config.footer.newsletter}
				</p>
				<div class="mt-5 flex gap-3">
					<a href="/semos1" class="flex h-9 w-9 items-center justify-center rounded-full bg-white/8 text-white/50 transition-all duration-200 hover:bg-[#6b7aff]/20 hover:text-[#6b7aff]" aria-label="LinkedIn">
						<svg class="h-4 w-4" fill="currentColor" viewBox="0 0 24 24"><path d="M20.447 20.452h-3.554v-5.569c0-1.328-.027-3.037-1.852-3.037-1.853 0-2.136 1.445-2.136 2.939v5.667H9.351V9h3.414v1.561h.046c.477-.9 1.637-1.85 3.37-1.85 3.601 0 4.267 2.37 4.267 5.455v6.286zM5.337 7.433a2.062 2.062 0 0 1-2.063-2.065 2.064 2.064 0 1 1 2.063 2.065zm1.782 13.019H3.555V9h3.564v11.452zM22.225 0H1.771C.792 0 0 .774 0 1.729v20.542C0 23.227.792 24 1.771 24h20.451C23.2 24 24 23.227 24 22.271V1.729C24 .774 23.2 0 22.222 0h.003z"/></svg>
					</a>
					<a href="/semos1" class="flex h-9 w-9 items-center justify-center rounded-full bg-white/8 text-white/50 transition-all duration-200 hover:bg-[#6b7aff]/20 hover:text-[#6b7aff]" aria-label="Twitter / X">
						<svg class="h-4 w-4" fill="currentColor" viewBox="0 0 24 24"><path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/></svg>
					</a>
					<a href="/semos1" aria-label="GitHub">
						<svg class="h-4 w-4" fill="currentColor" viewBox="0 0 24 24"><path d="M12 0C5.374 0 0 5.373 0 12c0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23A11.509 11.509 0 0 1 12 5.803c1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576C20.566 21.797 24 17.3 24 12c0-6.627-5.373-12-12-12z"/></svg>
					</a>
				</div>
				<div class="mt-6">
					<a
						href="mailto:{config.footer.email}"
						class="text-sm text-white/60 underline underline-offset-2 decoration-white/20 transition-colors hover:text-white hover:decoration-white/50"
					>
						{config.footer.email}
					</a>
				</div>
			</div>
		</div>
	</div>

	<!-- Copyright bar -->
	<div class="border-t border-white/8">
		<div class="mx-auto flex max-w-7xl flex-col items-center justify-between gap-2 px-6 py-6 text-xs text-white/35 md:flex-row">
			<p>&copy; 2026 {config.branding.logo_text}. All rights reserved.</p>
			<p>{config.branding.powered_by}</p>
		</div>
	</div>
</footer>

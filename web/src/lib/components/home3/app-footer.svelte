<script lang="ts">
	import BrainCircuitIcon from '@lucide/svelte/icons/brain-circuit';
	import GithubIcon       from '@lucide/svelte/icons/github';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import HeartIcon        from '@lucide/svelte/icons/heart';
	import ActivityIcon     from '@lucide/svelte/icons/activity';
	import ShieldCheckIcon  from '@lucide/svelte/icons/shield-check';
	import ZapIcon          from '@lucide/svelte/icons/zap';

	let { darkMode = true }: { darkMode: boolean } = $props();

	// --- Typography ---
	const fontMono = "'Fira Code', 'Cascadia Code', monospace"; // monospace for version / tech labels

	// --- Design tokens ---
	let cardBg      = $derived(darkMode ? '#1F2333' : '#FFFFFF');  // footer background (card surface)
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');  // border / divider
	let accent      = $derived(darkMode ? '#818CF8' : '#6366F1');  // primary accent (indigo)
	let accentTint  = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)'); // tint
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827'); // headings
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280'); // body text
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF'); // placeholder / disabled

	const footerLinks = [
		{
			title: 'Product',
			links: [
				{ label: 'Dashboard',      href: '/dashboard' },
				{ label: 'Agents',         href: '/agents' },
				{ label: 'Skills',         href: '/skills' },
				{ label: 'Applications',   href: '/applications' },
				{ label: 'Knowledge System', href: '/knowledge' }
			]
		},
		{
			title: 'Resources',
			links: [
				{ label: 'Documentation', href: '/docs' },
				{ label: 'API Reference', href: '/api' },
				{ label: 'Tutorials',     href: '/tutorials' },
				{ label: 'Changelog',     href: '/changelog' },
				{ label: 'Release Notes', href: '/releases' }
			]
		},
		{
			title: 'Support',
			links: [
				{ label: 'Help Center',      href: '/help' },
				{ label: 'Community Forum',  href: '/community' },
				{ label: 'GitHub Issues',    href: 'https://github.com/issues' },
				{ label: 'Feature Requests', href: '/features' },
				{ label: 'Contact Us',       href: '/contact' }
			]
		},
		{
			title: 'Legal',
			links: [
				{ label: 'Privacy Policy',  href: '/privacy' },
				{ label: 'Terms of Service', href: '/terms' },
				{ label: 'Cookie Policy',   href: '/cookies' },
				{ label: 'Security',        href: '/security' },
				{ label: 'Compliance',      href: '/compliance' }
			]
		}
	];

	const statusItems = [
		{ label: 'API',     status: 'operational', icon: ZapIcon },
		{ label: 'Agents',  status: 'operational', icon: ActivityIcon },
		{ label: 'Storage', status: 'operational', icon: ShieldCheckIcon }
	];

	function statusColor(status: string): string {
		return status === 'operational' ? '#34D399' : status === 'degraded' ? '#FBBF24' : '#F87171';
	}
</script>

<footer class="flex-shrink-0" style="background:{cardBg}; border-top:1px solid {borderColor};">

	<!-- Main footer content -->
	<div class="px-6 py-6">
		<div class="flex gap-8">

			<!-- Brand column -->
			<div class="flex-shrink-0 w-56">
				<div class="flex items-center gap-2 mb-3">
					<div
						class="flex items-center justify-center w-7 h-7 rounded-lg"
						style="background:{accentTint}; border:1px solid {accent}30;"
					>
						<BrainCircuitIcon class="w-4 h-4" style="color:{accent};" />
					</div>
					<span class="font-bold text-sm" style="color:{textPrimary}; font-family:{fontMono};">MyAIAssistant</span>
				</div>
				<p class="text-xs leading-relaxed mb-4" style="color:{textMuted};">
					Your intelligent AI workspace for the modern developer. Automate, assist, and amplify your daily workflow.
				</p>
				<!-- System status mini-widget -->
				<div class="rounded-lg p-3" style="background:{darkMode ? '#171B26' : '#F2F4F7'}; border:1px solid {borderColor};">
					<div class="text-xs font-semibold mb-2 uppercase tracking-wider" style="color:{textMuted}; font-family:{fontMono};">
						System Status
					</div>
					<div class="space-y-1.5">
						{#each statusItems as item}
							<div class="flex items-center gap-2">
								<div class="w-1.5 h-1.5 rounded-full" style="background:{statusColor(item.status)};"></div>
								<item.icon class="w-3 h-3" style="color:{textMuted};" />
								<span class="text-xs" style="color:{textMuted};">{item.label}</span>
								<span class="text-xs ml-auto" style="color:{statusColor(item.status)};">{item.status}</span>
							</div>
						{/each}
					</div>
				</div>
			</div>

			<!-- Spacer -->
			<div class="flex-1"></div>

			<!-- Link columns -->
			{#each footerLinks as col}
				<div class="min-w-28">
					<div class="text-xs font-semibold uppercase tracking-widest mb-3" style="color:{textMuted}; font-family:{fontMono};">
						{col.title}
					</div>
					<ul class="space-y-1.5">
						{#each col.links as link}
							<li>
								<a
									href={link.href}
									class="text-xs transition-colors duration-150"
									style="color:{textSecondary}; text-decoration:none;"
									onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = accent; }}
									onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; }}
								>
									{link.label}
								</a>
							</li>
						{/each}
					</ul>
				</div>
			{/each}
		</div>
	</div>

	<!-- Bottom bar -->
	<div class="px-6 py-3 flex items-center justify-between" style="border-top:1px solid {borderColor};">
		<div class="flex items-center gap-1.5 text-xs" style="color:{textMuted};">
			<span>© 2026 MyAIAssistant. Built with</span>
			<HeartIcon class="w-3 h-3" style="color:#F87171;" />
			<span>using</span>
			<span style="color:{accent}; font-family:{fontMono};">SvelteKit</span>
			<span>+</span>
			<span style="color:#818CF8; font-family:{fontMono};">Go</span>
			<span>+</span>
			<span style="color:#06b6d4; font-family:{fontMono};">Claude API</span>
		</div>
		<div class="flex items-center gap-4">
			<span class="text-xs" style="color:{textMuted}; font-family:{fontMono};">v3.0.0-rc1</span>
			<a
				href="https://github.com"
				class="flex items-center gap-1 text-xs transition-colors duration-150"
				style="color:{textMuted}; text-decoration:none;"
				onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = textPrimary; }}
				onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; }}
			>
				<GithubIcon class="w-3.5 h-3.5" />
				Source
				<ExternalLinkIcon class="w-2.5 h-2.5" />
			</a>
			<a
				href="/changelog"
				class="flex items-center gap-1 text-xs transition-colors duration-150"
				style="color:{textSecondary}; text-decoration:none;"
				onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = accent; }}
				onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; }}
			>
				Changelog
				<ExternalLinkIcon class="w-2.5 h-2.5" />
			</a>
		</div>
	</div>
</footer>

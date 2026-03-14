<script lang="ts">
	import BrainCircuitIcon from '@lucide/svelte/icons/brain-circuit';
	import GithubIcon from '@lucide/svelte/icons/github';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import HeartIcon from '@lucide/svelte/icons/heart';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import ShieldCheckIcon from '@lucide/svelte/icons/shield-check';
	import ZapIcon from '@lucide/svelte/icons/zap';

	let { darkMode = true }: { darkMode: boolean } = $props();

	let bg = $derived(darkMode ? '#0F172A' : '#ffffff');
	let border = $derived(darkMode ? '#1E293B' : '#e2e8f0');
	let textMain = $derived(darkMode ? '#F8FAFC' : '#0f172a');
	let textMuted = $derived(darkMode ? '#64748b' : '#94a3b8');
	let textSub = $derived(darkMode ? '#94a3b8' : '#64748b');

	const footerLinks = [
		{
			title: 'Product',
			links: [
				{ label: 'Dashboard', href: '#' },
				{ label: 'Agents', href: '#' },
				{ label: 'Skills', href: '#' },
				{ label: 'Applications', href: '#' },
				{ label: 'Knowledge Base', href: '#' }
			]
		},
		{
			title: 'Resources',
			links: [
				{ label: 'Documentation', href: '#' },
				{ label: 'API Reference', href: '#' },
				{ label: 'Tutorials', href: '#' },
				{ label: 'Changelog', href: '#' },
				{ label: 'Release Notes', href: '#' }
			]
		},
		{
			title: 'Support',
			links: [
				{ label: 'Help Center', href: '#' },
				{ label: 'Community Forum', href: '#' },
				{ label: 'GitHub Issues', href: '#' },
				{ label: 'Feature Requests', href: '#' },
				{ label: 'Contact Us', href: '#' }
			]
		},
		{
			title: 'Legal',
			links: [
				{ label: 'Privacy Policy', href: '#' },
				{ label: 'Terms of Service', href: '#' },
				{ label: 'Cookie Policy', href: '#' },
				{ label: 'Security', href: '#' },
				{ label: 'Compliance', href: '#' }
			]
		}
	];

	const statusItems = [
		{ label: 'API', status: 'operational', icon: ZapIcon },
		{ label: 'Agents', status: 'operational', icon: ActivityIcon },
		{ label: 'Storage', status: 'operational', icon: ShieldCheckIcon }
	];

	function statusColor(status: string) {
		return status === 'operational' ? '#22C55E' : status === 'degraded' ? '#f59e0b' : '#f43f5e';
	}
</script>

<footer
	class="flex-shrink-0"
	style="background:{bg}; border-top:1px solid {border};"
>
	<!-- Main footer content -->
	<div class="px-6 py-6">
		<div class="flex gap-8">
			<!-- Brand column -->
			<div class="flex-shrink-0 w-56">
				<div class="flex items-center gap-2 mb-3">
					<div
						class="flex items-center justify-center w-7 h-7 rounded-lg"
						style="background:rgba(34,197,94,0.15); border:1px solid rgba(34,197,94,0.3);"
					>
						<BrainCircuitIcon class="w-4 h-4" style="color:#22C55E;" />
					</div>
					<span class="font-bold text-sm" style="color:{textMain}; font-family:'Fira Code',monospace;">
						MyAIAssistant
					</span>
				</div>
				<p class="text-xs leading-relaxed mb-4" style="color:{textMuted};">
					Your intelligent AI workspace for the modern developer. Automate, assist, and amplify your daily workflow.
				</p>
				<!-- System status -->
				<div class="rounded-lg p-3" style="background:{darkMode ? '#020617' : '#f1f5f9'}; border:1px solid {border};">
					<div class="text-xs font-semibold mb-2 uppercase tracking-wider" style="color:{textMuted}; font-family:'Fira Code',monospace;">
						System Status
					</div>
					<div class="space-y-1.5">
						{#each statusItems as item}
							<div class="flex items-center gap-2">
								<div class="w-1.5 h-1.5 rounded-full" style="background:{statusColor(item.status)};"></div>
								<item.icon class="w-3 h-3" style="color:{textMuted};" />
								<span class="text-xs" style="color:{textSub};">{item.label}</span>
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
					<div class="text-xs font-semibold uppercase tracking-widest mb-3" style="color:{textMuted}; font-family:'Fira Code',monospace;">
						{col.title}
					</div>
					<ul class="space-y-1.5">
						{#each col.links as link}
							<li>
								<a
									href={link.href}
									class="text-xs transition-colors duration-150"
									style="color:{textSub}; text-decoration:none;"
									onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = '#22C55E'; }}
									onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textSub; }}
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
	<div
		class="px-6 py-3 flex items-center justify-between"
		style="border-top:1px solid {border};"
	>
		<div class="flex items-center gap-1.5 text-xs" style="color:{textMuted};">
			<span>© 2026 MyAIAssistant. Built with</span>
			<HeartIcon class="w-3 h-3" style="color:#f43f5e;" />
			<span>using</span>
			<span style="color:#22C55E; font-family:'Fira Code',monospace;">SvelteKit</span>
			<span>+</span>
			<span style="color:#818cf8; font-family:'Fira Code',monospace;">Go</span>
			<span>+</span>
			<span style="color:#06b6d4; font-family:'Fira Code',monospace;">Claude API</span>
		</div>

		<div class="flex items-center gap-4">
			<span class="text-xs" style="color:{textMuted}; font-family:'Fira Code',monospace;">v2.0.0-rc1</span>
			<a
				href="https://github.com"
				class="flex items-center gap-1 text-xs transition-colors duration-150"
				style="color:{textMuted}; text-decoration:none;"
				onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = textMain; }}
				onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; }}
			>
				<GithubIcon class="w-3.5 h-3.5" />
				Source
				<ExternalLinkIcon class="w-2.5 h-2.5" />
			</a>
			<a
				href="#"
				class="flex items-center gap-1 text-xs transition-colors duration-150"
				style="color:{textMuted}; text-decoration:none;"
				onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = '#22C55E'; }}
				onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; }}
			>
				Changelog
				<ExternalLinkIcon class="w-2.5 h-2.5" />
			</a>
		</div>
	</div>
</footer>

<script lang="ts">
	import DashboardView from './dashboard-view.svelte';
	import AppFooter from './app-footer.svelte';
	import BotIcon from '@lucide/svelte/icons/bot';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import AppWindowIcon from '@lucide/svelte/icons/app-window';
	import CodeIcon from '@lucide/svelte/icons/code-2';
	import UserIcon from '@lucide/svelte/icons/user';
	import BookOpenIcon from '@lucide/svelte/icons/book-open';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import InfoIcon from '@lucide/svelte/icons/info';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import SearchIcon from '@lucide/svelte/icons/search';

	export type ActiveSelection = {
		itemId: string;
		childId?: string;
		itemTitle: string;
		childTitle?: string;
	};

	let {
		activeMenu = null,
		darkMode = true
	}: {
		activeMenu: ActiveSelection | null;
		darkMode: boolean;
	} = $props();

	let surface = $derived(darkMode ? '#0F172A' : '#ffffff');
	let surface2 = $derived(darkMode ? '#1E293B' : '#f1f5f9');
	let border = $derived(darkMode ? '#1E293B' : '#e2e8f0');
	let textMain = $derived(darkMode ? '#F8FAFC' : '#0f172a');
	let textMuted = $derived(darkMode ? '#64748b' : '#94a3b8');

	// Placeholder items for different sections
	const agentItems = [
		{ name: 'ResearchBot', status: 'active', desc: 'Web research and summarization', model: 'Claude Sonnet 4.6' },
		{ name: 'SchedulerBot', status: 'active', desc: 'Task scheduling and calendar management', model: 'GPT-4o' },
		{ name: 'CodeReviewBot', status: 'idle', desc: 'Automated code review and suggestions', model: 'Claude Sonnet 4.6' },
		{ name: 'WriterBot', status: 'idle', desc: 'Technical writing and documentation', model: 'Claude Sonnet 4.6' }
	];

	const skillItems = [
		{ name: 'PDF Extractor', category: 'Document', usage: 142 },
		{ name: 'Web Scraper', category: 'Research', usage: 89 },
		{ name: 'Code Formatter', category: 'Dev', usage: 204 },
		{ name: 'Summarizer', category: 'NLP', usage: 317 },
		{ name: 'Email Composer', category: 'Comms', usage: 76 },
		{ name: 'Data Analyzer', category: 'Analytics', usage: 51 }
	];

	const appItems = [
		{ name: 'Notion Sync', icon: '📝', category: 'Productivity', status: 'connected' },
		{ name: 'GitHub', icon: '🐙', category: 'Dev', status: 'connected' },
		{ name: 'Slack', icon: '💬', category: 'Comms', status: 'connected' },
		{ name: 'Linear', icon: '📐', category: 'Project', status: 'disconnected' },
		{ name: 'Figma', icon: '🎨', category: 'Design', status: 'disconnected' },
		{ name: 'Jira', icon: '🎯', category: 'Project', status: 'disconnected' }
	];

	const kbDocuments = [
		{ name: 'Architecture Overview.pdf', size: '2.4 MB', updated: '2h ago', type: 'pdf' },
		{ name: 'API Reference v3.md', size: '156 KB', updated: '1d ago', type: 'md' },
		{ name: 'Meeting Notes Q1.docx', size: '48 KB', updated: '3d ago', type: 'docx' },
		{ name: 'Product Roadmap.pdf', size: '1.1 MB', updated: '1w ago', type: 'pdf' },
		{ name: 'Team Handbook.md', size: '92 KB', updated: '2w ago', type: 'md' }
	];

	// Determine what to render
	let sectionId = $derived(activeMenu?.itemId ?? 'dashboard');
	let childId = $derived(activeMenu?.childId);
	let title = $derived(activeMenu?.childTitle ?? activeMenu?.itemTitle ?? 'Dashboard');
</script>

<main
	class="flex-1 overflow-y-auto"
	style="background:{darkMode ? '#020617' : '#f8fafc'}; min-width:0;"
>
	<!-- min-height ensures the content area is always tall enough to scroll -->
	<div style="min-height:1200px;">
	{#if !activeMenu || sectionId === 'dashboard'}
		<DashboardView {darkMode} />

	{:else if sectionId === 'agents'}
		<div class="p-6 space-y-6">
			<div class="flex items-center justify-between">
				<div>
					<h2 class="text-xl font-semibold" style="color:{textMain}; font-family:'Fira Code',monospace;">
						{title}
					</h2>
					<p class="text-sm mt-1" style="color:{textMuted};">Manage and monitor your AI agents</p>
				</div>
				<button
					class="flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium cursor-pointer transition-all duration-200"
					style="background:#22C55E; color:#020617;"
					onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = '#16a34a'; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = '#22C55E'; }}
				>
					<PlusIcon class="w-4 h-4" />
					New Agent
				</button>
			</div>
			<div class="grid gap-4 md:grid-cols-2">
				{#each agentItems as agent}
					<div
						class="rounded-xl p-4 cursor-pointer transition-all duration-200"
						style="background:{surface}; border:1px solid {border};"
						onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.borderColor = '#22C55E40'; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.borderColor = border; }}
					>
						<div class="flex items-start justify-between mb-3">
							<div class="flex items-center gap-2">
								<div class="w-8 h-8 rounded-lg flex items-center justify-center" style="background:rgba(34,197,94,0.15);">
									<BotIcon class="w-4 h-4" style="color:#22C55E;" />
								</div>
								<div>
									<div class="text-sm font-semibold" style="color:{textMain};">{agent.name}</div>
									<div class="text-xs" style="color:{textMuted}; font-family:'Fira Code',monospace;">{agent.model}</div>
								</div>
							</div>
							<span
								class="px-2 py-0.5 text-xs rounded-full font-medium"
								style="background:{agent.status === 'active' ? 'rgba(34,197,94,0.15)' : 'rgba(100,116,139,0.15)'}; color:{agent.status === 'active' ? '#22C55E' : '#64748b'};"
							>
								{agent.status}
							</span>
						</div>
						<p class="text-sm" style="color:{textMuted};">{agent.desc}</p>
					</div>
				{/each}
			</div>
		</div>

	{:else if sectionId === 'skills'}
		<div class="p-6 space-y-6">
			<div class="flex items-center justify-between">
				<div>
					<h2 class="text-xl font-semibold" style="color:{textMain}; font-family:'Fira Code',monospace;">{title}</h2>
					<p class="text-sm mt-1" style="color:{textMuted};">Reusable AI capabilities and tools</p>
				</div>
				<button
					class="flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium cursor-pointer"
					style="background:#22C55E; color:#020617;"
				>
					<PlusIcon class="w-4 h-4" />
					New Skill
				</button>
			</div>
			<div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
				{#each skillItems as skill}
					<div
						class="rounded-xl p-4 cursor-pointer transition-all duration-200"
						style="background:{surface}; border:1px solid {border};"
						onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.borderColor = '#f59e0b40'; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.borderColor = border; }}
					>
						<div class="flex items-start justify-between mb-2">
							<div class="w-7 h-7 rounded-md flex items-center justify-center" style="background:rgba(245,158,11,0.15);">
								<ZapIcon class="w-3.5 h-3.5" style="color:#f59e0b;" />
							</div>
							<span class="text-xs px-2 py-0.5 rounded-full" style="background:{surface2}; color:{textMuted};">
								{skill.category}
							</span>
						</div>
						<div class="text-sm font-semibold mt-2" style="color:{textMain};">{skill.name}</div>
						<div class="text-xs mt-1" style="color:{textMuted};">{skill.usage} uses this month</div>
					</div>
				{/each}
			</div>
		</div>

	{:else if sectionId === 'applications'}
		<div class="p-6 space-y-6">
			<div class="flex items-center justify-between">
				<div>
					<h2 class="text-xl font-semibold" style="color:{textMain}; font-family:'Fira Code',monospace;">{title}</h2>
					<p class="text-sm mt-1" style="color:{textMuted};">Connected apps and integrations</p>
				</div>
				<div class="flex items-center gap-2">
					<div class="relative">
						<SearchIcon class="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2" style="color:{textMuted};" />
						<input
							type="text"
							placeholder="Search apps..."
							class="pl-9 pr-4 py-2 rounded-lg text-sm outline-none"
							style="background:{surface}; border:1px solid {border}; color:{textMain}; width:200px;"
						/>
					</div>
				</div>
			</div>
			<div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
				{#each appItems as app}
					<div
						class="rounded-xl p-4 cursor-pointer transition-all duration-200"
						style="background:{surface}; border:1px solid {border};"
						onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.borderColor = '#818cf840'; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.borderColor = border; }}
					>
						<div class="flex items-center gap-3 mb-2">
							<span class="text-2xl">{app.icon}</span>
							<div>
								<div class="text-sm font-semibold" style="color:{textMain};">{app.name}</div>
								<div class="text-xs" style="color:{textMuted};">{app.category}</div>
							</div>
						</div>
						<div class="flex items-center justify-between mt-3">
							<span class="text-xs" style="color:{app.status === 'connected' ? '#22C55E' : textMuted};">
								{app.status === 'connected' ? '● Connected' : '○ Not connected'}
							</span>
							<button
								class="text-xs px-2 py-1 rounded-md cursor-pointer"
								style="background:{surface2}; color:{textMuted};"
							>
								{app.status === 'connected' ? 'Manage' : 'Connect'}
							</button>
						</div>
					</div>
				{/each}
			</div>
		</div>

	{:else if sectionId === 'coding'}
		<div class="p-6 space-y-6">
			<h2 class="text-xl font-semibold" style="color:{textMain}; font-family:'Fira Code',monospace;">{title}</h2>
			<div
				class="rounded-xl p-6"
				style="background:{surface}; border:1px solid {border};"
			>
				<div class="flex items-center gap-3 mb-4">
					<div class="w-10 h-10 rounded-xl flex items-center justify-center" style="background:rgba(245,158,11,0.15);">
						<CodeIcon class="w-5 h-5" style="color:#f59e0b;" />
					</div>
					<div>
						<div class="font-semibold" style="color:{textMain};">Coding Assistant</div>
						<div class="text-sm" style="color:{textMuted};">
							{childId === 'coding-review' ? 'Automated code review powered by Claude' :
							 childId === 'coding-generate' ? 'Generate code from natural language' :
							 childId === 'coding-debug' ? 'Debug errors with AI assistance' :
							 'Refactor and improve existing code'}
						</div>
					</div>
				</div>
				<div
					class="rounded-lg p-4 font-mono text-sm"
					style="background:{darkMode ? '#020617' : '#f8fafc'}; border:1px solid {border}; color:{textMuted};"
				>
					<span style="color:#22C55E;"># </span>
					<span style="color:{textMain};">Connect your repository to get started...</span>
					<br />
					<span style="color:{textMuted}; opacity:0.5;">$ git remote add origin &lt;your-repo&gt;</span>
				</div>
			</div>
		</div>

	{:else if sectionId === 'personal'}
		<div class="p-6 space-y-6">
			<h2 class="text-xl font-semibold" style="color:{textMain}; font-family:'Fira Code',monospace;">{title}</h2>
			<div class="grid gap-4 md:grid-cols-2">
				{#each ['Write a summary of today\'s meetings', 'Review Q1 roadmap doc', 'Prepare code review for PR #142', 'Send weekly status update'] as task, i}
					<div
						class="flex items-start gap-3 rounded-xl p-4"
						style="background:{surface}; border:1px solid {border};"
					>
						<div class="mt-0.5 w-4 h-4 rounded flex-shrink-0" style="border:2px solid {border};"></div>
						<div>
							<div class="text-sm" style="color:{textMain};">{task}</div>
							<div class="text-xs mt-1" style="color:{textMuted};">Priority {i + 1}</div>
						</div>
					</div>
				{/each}
			</div>
		</div>

	{:else if sectionId === 'knowledge'}
		<div class="p-6 space-y-6">
			<div class="flex items-center justify-between">
				<div>
					<h2 class="text-xl font-semibold" style="color:{textMain}; font-family:'Fira Code',monospace;">{title}</h2>
					<p class="text-sm mt-1" style="color:{textMuted};">234 documents indexed</p>
				</div>
				<button class="flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium cursor-pointer" style="background:#22C55E; color:#020617;">
					<PlusIcon class="w-4 h-4" />Import
				</button>
			</div>
			<div class="relative mb-2">
				<SearchIcon class="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2" style="color:{textMuted};" />
				<input
					type="text"
					placeholder="Search your knowledge base..."
					class="w-full pl-9 pr-4 py-2.5 rounded-xl text-sm outline-none"
					style="background:{surface}; border:1px solid {border}; color:{textMain};"
				/>
			</div>
			<div class="rounded-xl overflow-hidden" style="border:1px solid {border};">
				{#each kbDocuments as doc, i}
					<div
						class="flex items-center gap-3 px-4 py-3 cursor-pointer transition-colors duration-150"
						style="background:{i % 2 === 0 ? surface : surface2 + '80'}; border-bottom:{i < kbDocuments.length - 1 ? `1px solid ${border}` : 'none'};"
						onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = surface2; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = i % 2 === 0 ? surface : surface2 + '80'; }}
					>
						<BookOpenIcon class="w-4 h-4 flex-shrink-0" style="color:#06b6d4;" />
						<div class="flex-1 min-w-0">
							<div class="text-sm font-medium truncate" style="color:{textMain};">{doc.name}</div>
						</div>
						<div class="text-xs flex-shrink-0" style="color:{textMuted}; font-family:'Fira Code',monospace;">{doc.size}</div>
						<div class="text-xs flex-shrink-0" style="color:{textMuted};">{doc.updated}</div>
					</div>
				{/each}
			</div>
		</div>

	{:else if sectionId === 'settings'}
		<div class="p-6 space-y-6">
			<h2 class="text-xl font-semibold" style="color:{textMain}; font-family:'Fira Code',monospace;">Settings</h2>
			{#each [
				{ label: 'General', items: ['Language & Region', 'Timezone', 'Notifications', 'Auto-save'] },
				{ label: 'AI Models', items: ['Default Model', 'Fallback Model', 'Token Limits', 'Temperature'] },
				{ label: 'Integrations', items: ['API Keys', 'OAuth Apps', 'Webhooks', 'SSO'] },
				{ label: 'Security', items: ['2FA', 'Session Management', 'Audit Log', 'Data Export'] }
			] as section}
				{#if !childId || childId === 'settings-' + section.label.toLowerCase().replace(' ', '-')}
					<div class="rounded-xl" style="border:1px solid {border}; overflow:hidden;">
						<div class="px-4 py-3" style="background:{surface2}; border-bottom:1px solid {border};">
							<div class="text-sm font-semibold" style="color:{textMain};">{section.label}</div>
						</div>
						{#each section.items as item, i}
							<div
								class="flex items-center justify-between px-4 py-3 cursor-pointer"
								style="background:{surface}; border-bottom:{i < section.items.length - 1 ? `1px solid ${border}` : 'none'};"
								onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = surface2; }}
								onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = surface; }}
							>
								<span class="text-sm" style="color:{textMain};">{item}</span>
								<SettingsIcon class="w-3.5 h-3.5" style="color:{textMuted};" />
							</div>
						{/each}
					</div>
				{/if}
			{/each}
		</div>

	{:else if sectionId === 'about'}
		<div class="p-6 space-y-6">
			<h2 class="text-xl font-semibold" style="color:{textMain}; font-family:'Fira Code',monospace;">About</h2>
			<div class="rounded-xl p-6 space-y-4" style="background:{surface}; border:1px solid {border};">
				<div class="text-center mb-6">
					<div class="text-3xl font-bold" style="color:#22C55E; font-family:'Fira Code',monospace;">MyAIAssistant</div>
					<div class="text-sm mt-1" style="color:{textMuted};">Version 2.0.0</div>
				</div>
				{#each [
					{ label: 'Build', value: '2026.03.10-rc1' },
					{ label: 'Engine', value: 'SvelteKit 2 + Go Echo' },
					{ label: 'AI Runtime', value: 'Anthropic Claude API' },
					{ label: 'License', value: 'MIT' }
				] as row}
					<div class="flex items-center justify-between py-2" style="border-bottom:1px solid {border};">
						<span class="text-sm" style="color:{textMuted};">{row.label}</span>
						<span class="text-sm font-medium" style="color:{textMain}; font-family:'Fira Code',monospace;">{row.value}</span>
					</div>
				{/each}
			</div>
		</div>

	{:else}
		<!-- Generic placeholder for unknown sections -->
		<div class="flex flex-col items-center justify-center h-full gap-4">
			<div class="w-16 h-16 rounded-2xl flex items-center justify-center" style="background:rgba(34,197,94,0.1); border:1px solid rgba(34,197,94,0.2);">
				<InfoIcon class="w-8 h-8" style="color:#22C55E;" />
			</div>
			<div class="text-lg font-semibold" style="color:{textMain};">{title}</div>
			<p class="text-sm" style="color:{textMuted};">This section is coming soon.</p>
		</div>
	{/if}
	</div><!-- end min-height wrapper -->

	<!-- Footer floats after the content when scrolled -->
	<AppFooter {darkMode} />
</main>

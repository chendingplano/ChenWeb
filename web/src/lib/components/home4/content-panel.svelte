<script lang="ts">
	import DashboardView from './dashboard-view.svelte';
	import AppFooter from './app-footer.svelte';
	import BotIcon from '@lucide/svelte/icons/bot';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import LayoutGridIcon from '@lucide/svelte/icons/layout-grid';
	import CodeIcon from '@lucide/svelte/icons/code-2';
	import BookOpenIcon from '@lucide/svelte/icons/book-open';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import SearchIcon from '@lucide/svelte/icons/search';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import HomeIcon from '@lucide/svelte/icons/home';

	type ActiveSelection = {
		itemId: string;
		childId?: string;
		itemTitle: string;
		childTitle?: string;
	};

	let {
		darkMode,
		activeMenu,
		drawerOpen,
		drawerWidth
	}: {
		darkMode: boolean;
		activeMenu: ActiveSelection | null;
		drawerOpen: boolean;
		drawerWidth: number;
	} = $props();

	// --- Layout constants (must match +page.svelte) ---
	const HERO_HEADER_HEIGHT   = 200; // must match +page.svelte
	const NAV_BAR_HEIGHT       = 44;  // must match +page.svelte
	const DRAWER_ANIM_DURATION = '250ms'; // must match right-drawer

	// Suppress unused constant warnings
	void HERO_HEADER_HEIGHT; void NAV_BAR_HEIGHT;

	// --- Colour tokens (all $derived from darkMode) ---
	let pageBg      = $derived(darkMode ? '#171B26' : '#F2F4F7');       // page background
	let surface     = $derived(darkMode ? '#1E2535' : '#ffffff');       // card background
	let surface2    = $derived(darkMode ? '#252B3B' : '#f1f5f9');       // secondary surface
	let border      = $derived(darkMode ? '#2D3348' : '#E4E6EB');       // border / divider lines
	let textMain    = $derived(darkMode ? '#E2E8F0' : '#111827');       // primary text
	let textMuted   = $derived(darkMode ? '#64748b' : '#94a3b8');       // muted text
	let accent      = $derived(darkMode ? '#818CF8' : '#6366F1');       // primary accent (indigo)

	let sectionId = $derived(activeMenu?.itemId ?? 'dashboard');
	let childId   = $derived(activeMenu?.childId);
	let title     = $derived(activeMenu?.childTitle ?? activeMenu?.itemTitle ?? 'Dashboard');

	// Placeholder data (same as home2/content-panel but adapted for indigo palette)
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

	// Show breadcrumb when not on dashboard
	let showBreadcrumb = $derived(!!activeMenu && sectionId !== 'dashboard');

	// Computed padding-right based on drawer state
	let paddingRight = $derived(drawerOpen ? `${drawerWidth}px` : '0px');
</script>

<main
	class="flex-1 overflow-y-auto"
	style="
		background:{pageBg};
		min-height:calc(100vh - 244px);
		padding-right:{paddingRight};
		transition:padding-right {DRAWER_ANIM_DURATION} ease;
		min-width:0;
	"
>
	<div style="min-height:800px;">
		<!-- Breadcrumb (when not on dashboard) -->
		{#if showBreadcrumb}
			<div
				class="flex items-center gap-1.5 px-6 py-3 text-sm"
				style="border-bottom:1px solid {border}; background:{surface}20;"
			>
				<HomeIcon class="w-3.5 h-3.5 flex-shrink-0" style="color:{textMuted};" />
				<ChevronRightIcon class="w-3 h-3 flex-shrink-0" style="color:{textMuted};" />
				<span style="color:{activeMenu?.childId ? textMuted : accent};">{activeMenu?.itemTitle}</span>
				{#if activeMenu?.childTitle}
					<ChevronRightIcon class="w-3 h-3 flex-shrink-0" style="color:{textMuted};" />
					<span style="color:{accent};">{activeMenu.childTitle}</span>
				{/if}
			</div>
		{/if}

		{#if !activeMenu || sectionId === 'dashboard'}
			<DashboardView {darkMode} />

		{:else if sectionId === 'agents'}
			<div class="p-6 space-y-6">
				<div class="flex items-center justify-between">
					<div>
						<h2 class="text-xl font-semibold" style="color:{textMain}; font-family:'Fira Code',monospace;">{title}</h2>
						<p class="text-sm mt-1" style="color:{textMuted};">Manage and monitor your AI agents</p>
					</div>
					<button
						class="flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium cursor-pointer transition-all duration-200"
						style="background:{accent}; color:#ffffff;"
						onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.opacity = '0.9'; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.opacity = '1'; }}
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
							onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.borderColor = accent + '40'; }}
							onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.borderColor = border; }}
						>
							<div class="flex items-start justify-between mb-3">
								<div class="flex items-center gap-2">
									<div class="w-8 h-8 rounded-lg flex items-center justify-center" style="background:rgba(129,140,248,0.15);">
										<BotIcon class="w-4 h-4" style="color:{accent};" />
									</div>
									<div>
										<div class="text-sm font-semibold" style="color:{textMain};">{agent.name}</div>
										<div class="text-xs" style="color:{textMuted}; font-family:'Fira Code',monospace;">{agent.model}</div>
									</div>
								</div>
								<span
									class="px-2 py-0.5 text-xs rounded-full font-medium"
									style="background:{agent.status === 'active' ? 'rgba(129,140,248,0.15)' : 'rgba(100,116,139,0.15)'}; color:{agent.status === 'active' ? accent : '#64748b'};"
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
						style="background:{accent}; color:#ffffff;"
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
							onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.borderColor = accent + '40'; }}
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
				<div class="rounded-xl p-6" style="background:{surface}; border:1px solid {border};">
					<div class="flex items-center gap-3 mb-4">
						<div class="w-10 h-10 rounded-xl flex items-center justify-center" style="background:rgba(245,158,11,0.15);">
							<CodeIcon class="w-5 h-5" style="color:#f59e0b;" />
						</div>
						<div>
							<div class="font-semibold" style="color:{textMain};">Coding Assistant</div>
							<div class="text-sm" style="color:{textMuted};">
								{childId === 'coding-review' ? 'Automated code review powered by Claude' :
								 childId === 'coding-gen' ? 'Generate code from natural language' :
								 childId === 'coding-debug' ? 'Debug errors with AI assistance' :
								 'AI-powered coding tools'}
							</div>
						</div>
					</div>
					<div
						class="rounded-lg p-4 font-mono text-sm"
						style="background:{darkMode ? '#0D1117' : '#f8fafc'}; border:1px solid {border}; color:{textMuted};"
					>
						<span style="color:{accent};"># </span>
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
					{#each ["Write a summary of today's meetings", 'Review Q1 roadmap doc', 'Prepare code review for PR #142', 'Send weekly status update'] as task, i}
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
					<button class="flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium cursor-pointer" style="background:{accent}; color:#ffffff;">
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
				{/each}
			</div>

		{:else if sectionId === 'about'}
			<div class="p-6 space-y-6">
				<h2 class="text-xl font-semibold" style="color:{textMain}; font-family:'Fira Code',monospace;">About</h2>
				<div class="rounded-xl p-6 space-y-4" style="background:{surface}; border:1px solid {border};">
					<div class="text-center mb-6">
						<div class="text-3xl font-bold" style="color:{accent}; font-family:'Fira Code',monospace;">MyAIAssistant</div>
						<div class="text-sm mt-1" style="color:{textMuted};">Version 4.0.0</div>
					</div>
					{#each [
						{ label: 'Build', value: '2026.03.11-rc1' },
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
			<div class="flex flex-col items-center justify-center py-24 gap-4">
				<div class="w-16 h-16 rounded-2xl flex items-center justify-center" style="background:rgba(129,140,248,0.1); border:1px solid rgba(129,140,248,0.2);">
					<LayoutGridIcon class="w-8 h-8" style="color:{accent};" />
				</div>
				<div class="text-lg font-semibold" style="color:{textMain};">{title}</div>
				<p class="text-sm" style="color:{textMuted};">This section is coming soon.</p>
			</div>
		{/if}
	</div>

	<!-- Footer inside scroll area -->
	<div style="margin-top:48px;">
		<AppFooter {darkMode} />
	</div>
</main>

<script lang="ts">
	import Bot from '@lucide/svelte/icons/bot';
	import Puzzle from '@lucide/svelte/icons/puzzle';
	import Grid3X3 from '@lucide/svelte/icons/grid-3x3';
	import Code2 from '@lucide/svelte/icons/code-2';
	import User2 from '@lucide/svelte/icons/user-2';
	import LayoutDashboard from '@lucide/svelte/icons/layout-dashboard';
	import Settings from '@lucide/svelte/icons/settings';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import Cpu from '@lucide/svelte/icons/cpu';
	import Zap from '@lucide/svelte/icons/zap';
	import Star from '@lucide/svelte/icons/star';
	import Activity from '@lucide/svelte/icons/activity';
	import Bell from '@lucide/svelte/icons/bell';
	import Search from '@lucide/svelte/icons/search';
	import Moon from '@lucide/svelte/icons/moon';
	import Sun from '@lucide/svelte/icons/sun';
	import Layers from '@lucide/svelte/icons/layers';
	import Stethoscope from '@lucide/svelte/icons/stethoscope';
	import Scale from '@lucide/svelte/icons/scale';
	import BarChart3 from '@lucide/svelte/icons/bar-chart-3';
	import Globe from '@lucide/svelte/icons/globe';
	import Shield from '@lucide/svelte/icons/shield';
	import Rocket from '@lucide/svelte/icons/rocket';

	// ─── Types ────────────────────────────────────────────────────────────────
	type MenuLeaf = { id: string; label: string; icon: any };
	type MenuGroup = MenuLeaf & { children: MenuLeaf[] };
	type MenuItem = MenuLeaf | MenuGroup;

	function hasChildren(item: MenuItem): item is MenuGroup {
		return 'children' in item && Array.isArray((item as MenuGroup).children);
	}

	// ─── Menu definition ──────────────────────────────────────────────────────
	const menuItems: MenuItem[] = [
		{ id: 'dashboard', label: 'Dashboard', icon: LayoutDashboard },
		{
			id: 'agents',
			label: 'Agents',
			icon: Bot,
			children: [
				{ id: 'agents-all', label: 'All Agents', icon: Bot },
				{ id: 'agents-create', label: 'Create Agent', icon: Rocket },
				{ id: 'agents-marketplace', label: 'Marketplace', icon: Globe }
			]
		},
		{
			id: 'skills',
			label: 'Skills',
			icon: Puzzle,
			children: [
				{ id: 'skills-all', label: 'Skills Library', icon: Puzzle },
				{ id: 'skills-create', label: 'Create Skill', icon: Zap },
				{ id: 'skills-deployed', label: 'Deployed', icon: Shield }
			]
		},
		{
			id: 'ai-apps',
			label: 'AI Apps',
			icon: Grid3X3,
			children: [
				{ id: 'apps-healthcare', label: 'Healthcare', icon: Stethoscope },
				{ id: 'apps-finance', label: 'Finance', icon: BarChart3 },
				{ id: 'apps-legal', label: 'Legal', icon: Scale }
			]
		},
		{
			id: 'coding',
			label: 'Coding Assistants',
			icon: Code2,
			children: [
				{ id: 'coding-claude', label: 'Claude Code', icon: Code2 },
				{ id: 'coding-codex', label: 'Codex', icon: Code2 },
				{ id: 'coding-qwen', label: 'Qwen Code', icon: Code2 },
				{ id: 'coding-opencode', label: 'OpenCode', icon: Code2 }
			]
		},
		{
			id: 'personal',
			label: 'Personal Assistant',
			icon: User2,
			children: [{ id: 'personal-openclaw', label: 'OpenClaw', icon: User2 }]
		},
		{ id: 'settings', label: 'Settings', icon: Settings }
	];

	// ─── State ────────────────────────────────────────────────────────────────
	let activeItem = $state<string>('dashboard');
	let expandedItems = $state<Set<string>>(new Set(['coding', 'ai-apps']));
	let isDark = $state(false);

	function toggleExpand(id: string) {
		const next = new Set(expandedItems);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		expandedItems = next;
	}

	function selectItem(id: string, hasKids = false) {
		activeItem = id;
		if (hasKids) toggleExpand(id);
	}

	// ─── Dashboard data ───────────────────────────────────────────────────────
	const stats = [
		{ label: 'Active Agents', value: '12', change: '+3', icon: Bot, color: 'text-blue-500' },
		{ label: 'Skills Available', value: '48', change: '+8', icon: Puzzle, color: 'text-purple-500' },
		{ label: 'AI Apps', value: '24', change: '+2', icon: Grid3X3, color: 'text-emerald-500' },
		{ label: 'Coding Tools', value: '4', change: '0', icon: Code2, color: 'text-orange-500' }
	];

	const recentActivity = [
		{ time: '2 min ago', action: 'Claude Code assisted with Go refactoring', type: 'coding' },
		{ time: '15 min ago', action: 'OpenClaw completed daily digest', type: 'personal' },
		{ time: '1 hr ago', action: 'New skill "doc-coauthoring" deployed', type: 'skills' },
		{ time: '3 hrs ago', action: 'Qwen Code generated REST API endpoints', type: 'coding' },
		{ time: '5 hrs ago', action: 'Healthcare AI app processed 120 records', type: 'apps' },
		{ time: '8 hrs ago', action: 'Legal AI app reviewed 3 contracts', type: 'apps' }
	];

	const quickAccess = [
		{
			id: 'coding-claude',
			label: 'Claude Code',
			desc: 'AI-powered coding assistant',
			icon: Code2,
			accent: '#3b82f6'
		},
		{
			id: 'personal-openclaw',
			label: 'OpenClaw',
			desc: 'Personal digital assistant',
			icon: User2,
			accent: '#8b5cf6'
		},
		{
			id: 'agents',
			label: 'Agents Hub',
			desc: 'Manage all AI agents',
			icon: Bot,
			accent: '#10b981'
		},
		{
			id: 'skills',
			label: 'Skills Library',
			desc: 'Browse & deploy skills',
			icon: Puzzle,
			accent: '#f59e0b'
		}
	];

	const codingTools = [
		{
			id: 'coding-claude',
			label: 'Claude Code',
			desc: "Anthropic's AI coding CLI with full codebase awareness and agentic task execution.",
			icon: Code2,
			badge: 'Popular',
			badgeColor: 'bg-blue-100 text-blue-700'
		},
		{
			id: 'coding-codex',
			label: 'Codex',
			desc: "OpenAI's cloud-based coding agent for automated software engineering tasks.",
			icon: Code2,
			badge: 'Cloud',
			badgeColor: 'bg-green-100 text-green-700'
		},
		{
			id: 'coding-qwen',
			label: 'Qwen Code',
			desc: "Alibaba's open-source coding agent with strong multilingual code generation.",
			icon: Code2,
			badge: 'Open Source',
			badgeColor: 'bg-orange-100 text-orange-700'
		},
		{
			id: 'coding-opencode',
			label: 'OpenCode',
			desc: 'Community-driven coding agent with terminal UI and extensible plugin system.',
			icon: Code2,
			badge: 'Community',
			badgeColor: 'bg-purple-100 text-purple-700'
		}
	];

	const systemServices = [
		{ label: 'Claude API', status: 'Operational', dot: 'bg-emerald-400' },
		{ label: 'OpenAI API', status: 'Operational', dot: 'bg-emerald-400' },
		{ label: 'OpenClaw', status: 'Active', dot: 'bg-emerald-400' },
		{ label: 'OpenCode', status: 'Idle', dot: 'bg-amber-400' }
	];

	const trendingSkills = ['frontend-design', 'doc-coauthoring', 'mcp-builder', 'pdf', 'allium'];

	// ─── Helpers ──────────────────────────────────────────────────────────────
	function activityDot(type: string) {
		const map: Record<string, string> = {
			coding: 'bg-blue-400',
			personal: 'bg-purple-400',
			skills: 'bg-amber-400',
			apps: 'bg-emerald-400'
		};
		return map[type] ?? 'bg-slate-400';
	}

	$effect(() => {
		// Sync dark class to document for correct CSS variable inheritance
		if (typeof document !== 'undefined') {
			document.documentElement.classList.toggle('dark', isDark);
		}
	});
</script>

<!-- ══════════════════════════════════════════════════════════════════════════
     ROOT: full-viewport flex column
     ══════════════════════════════════════════════════════════════════════════ -->
<div class="flex h-screen flex-col overflow-hidden bg-background text-foreground">
	<!-- ╔══════════════════════════════════════════════════════════════════════
	     TOP PANEL — Header / Banner
	     ══════════════════════════════════════════════════════════════════════╗ -->
	<header
		class="relative z-10 flex shrink-0 items-center justify-between gap-4 bg-gradient-to-r from-slate-900 via-blue-950 to-indigo-900 px-6 py-3 shadow-lg"
	>
		<!-- Logo + name -->
		<div class="flex items-center gap-3">
			<div
				class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-blue-500 shadow-md ring-2 ring-blue-400/40"
			>
				<Cpu class="h-5 w-5 text-white" />
			</div>
			<div class="leading-none">
				<p class="text-lg font-bold tracking-tight text-white">My AI Assistant</p>
				<p class="text-[11px] text-blue-300">Comprehensive AI Hub</p>
			</div>
		</div>

		<!-- Centre slogan -->
		<div class="hidden flex-1 flex-col items-center text-center lg:flex">
			<p class="text-sm font-semibold text-blue-100">
				Agents · Skills · AI Apps · Coding Assistants · Personal AI
			</p>
			<p class="text-xs text-blue-400">One platform to orchestrate all your AI tools</p>
		</div>

		<!-- Action row -->
		<div class="flex items-center gap-1">
			<button
				class="rounded-lg p-2 text-blue-200 transition-colors hover:bg-white/10"
				aria-label="Search"
			>
				<Search class="h-4 w-4" />
			</button>
			<button
				class="rounded-lg p-2 text-blue-200 transition-colors hover:bg-white/10"
				aria-label="Notifications"
			>
				<Bell class="h-4 w-4" />
			</button>
			<button
				class="rounded-lg p-2 text-blue-200 transition-colors hover:bg-white/10"
				aria-label="Toggle dark mode"
				onclick={() => (isDark = !isDark)}
			>
				{#if isDark}
					<Sun class="h-4 w-4" />
				{:else}
					<Moon class="h-4 w-4" />
				{/if}
			</button>
			<div
				class="ml-2 flex h-8 w-8 items-center justify-center rounded-full bg-gradient-to-br from-blue-400 to-indigo-500 text-xs font-bold text-white shadow"
			>
				AI
			</div>
		</div>
	</header>

	<!-- ╔══════════════════════════════════════════════════════════════════════
	     MIDDLE ROW — Left menu + Content + Right panel
	     ══════════════════════════════════════════════════════════════════════╗ -->
	<div class="flex min-h-0 flex-1">
		<!-- ── LEFT PANEL: Navigation Menu ─────────────────────────────────── -->
		<nav class="flex w-60 shrink-0 flex-col overflow-y-auto border-r border-border bg-sidebar">
			<div class="flex-1 p-2">
				{#each menuItems as item (item.id)}
					{@const kids = hasChildren(item) ? item.children : null}
					{@const isActive = activeItem === item.id}
					{@const isExpanded = expandedItems.has(item.id)}
					{@const ItemIcon = item.icon}

					<div class="mb-0.5">
						<!-- Parent button -->
						<button
							class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors
								{isActive
								? 'bg-primary text-primary-foreground'
								: 'text-sidebar-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground'}"
							onclick={() => selectItem(item.id, !!kids)}
						>
							<ItemIcon class="h-4 w-4 shrink-0" />
							<span class="flex-1 text-left">{item.label}</span>
							{#if kids}
								{#if isExpanded}
									<ChevronDown class="h-3 w-3 shrink-0 opacity-60" />
								{:else}
									<ChevronRight class="h-3 w-3 shrink-0 opacity-60" />
								{/if}
							{/if}
						</button>

						<!-- Children -->
						{#if kids && isExpanded}
							<div class="ml-3 mt-0.5 space-y-0.5 border-l border-border pl-3">
								{#each kids as child (child.id)}
									{@const childActive = activeItem === child.id}
									<button
										class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-xs transition-colors
											{childActive
											? 'bg-primary/10 font-semibold text-primary'
											: 'text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground'}"
										onclick={() => selectItem(child.id)}
									>
										<span
											class="h-1.5 w-1.5 shrink-0 rounded-full {childActive
												? 'bg-primary'
												: 'bg-current opacity-40'}"
										></span>
										{child.label}
									</button>
								{/each}
							</div>
						{/if}
					</div>
				{/each}
			</div>

			<!-- Sidebar footer -->
			<div class="border-t border-border p-3">
				<div class="flex items-center gap-2 text-xs text-muted-foreground">
					<div class="h-1.5 w-1.5 rounded-full bg-emerald-400"></div>
					<span>All services online</span>
				</div>
			</div>
		</nav>

		<!-- ── MIDDLE PANEL: Content ─────────────────────────────────────────── -->
		<main class="flex-1 overflow-y-auto bg-muted/20 p-6">
			<!-- ─ DASHBOARD ─ -->
			{#if activeItem === 'dashboard'}
				<div class="space-y-6">
					<div>
						<h2 class="text-2xl font-bold">Dashboard</h2>
						<p class="text-sm text-muted-foreground">
							Welcome back — here's your AI ecosystem at a glance.
						</p>
					</div>

					<!-- Stats cards -->
					<div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
						{#each stats as s (s.label)}
						{@const StatIcon = s.icon}
							<div class="rounded-xl border border-border bg-card p-4 shadow-sm">
								<div class="flex items-center justify-between">
									<p class="text-xs font-medium text-muted-foreground">{s.label}</p>
									<StatIcon class="h-4 w-4 {s.color}" />
								</div>
								<p class="mt-2 text-2xl font-bold">{s.value}</p>
								<p class="mt-0.5 text-xs text-muted-foreground">
									<span class="text-emerald-500">{s.change}</span> this week
								</p>
							</div>
						{/each}
					</div>

					<!-- Quick access -->
					<div>
						<h3
							class="mb-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground"
						>
							Quick Access
						</h3>
						<div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
							{#each quickAccess as qa (qa.id)}
							{@const QaIcon = qa.icon}
								<button
									class="group rounded-xl border border-border bg-card p-4 text-left shadow-sm transition-all hover:-translate-y-0.5 hover:shadow-md"
									onclick={() => selectItem(qa.id)}
								>
									<div
										class="mb-2 flex h-9 w-9 items-center justify-center rounded-lg"
										style="background-color: {qa.accent}1a;"
									>
										<QaIcon class="h-5 w-5" style="color: {qa.accent}" />
									</div>
									<p class="text-sm font-semibold">{qa.label}</p>
									<p class="mt-0.5 text-xs text-muted-foreground">{qa.desc}</p>
								</button>
							{/each}
						</div>
					</div>

					<!-- Recent activity -->
					<div>
						<h3
							class="mb-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground"
						>
							Recent Activity
						</h3>
						<div class="divide-y divide-border rounded-xl border border-border bg-card shadow-sm">
							{#each recentActivity as a (a.time + a.action)}
								<div class="flex items-start gap-3 px-4 py-3">
									<span class="mt-1.5 h-2 w-2 shrink-0 rounded-full {activityDot(a.type)}"></span>
									<div class="min-w-0 flex-1">
										<p class="truncate text-sm">{a.action}</p>
										<p class="text-xs text-muted-foreground">{a.time}</p>
									</div>
								</div>
							{/each}
						</div>
					</div>
				</div>

			<!-- ─ AGENTS ─ -->
			{:else if activeItem === 'agents' || activeItem === 'agents-all'}
				<div class="space-y-4">
					<h2 class="text-2xl font-bold">Agents Hub</h2>
					<p class="text-muted-foreground">
						Create, deploy, and orchestrate AI agents for any workflow.
					</p>
					<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
						{#each ['Research Agent', 'Code Review Agent', 'Data Analysis Agent', 'Document Agent', 'Email Agent', 'Scheduler Agent'] as name, i}
							<div class="rounded-xl border border-border bg-card p-4 shadow-sm">
								<div class="mb-3 flex items-center justify-between">
									<div
										class="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-100 text-blue-600"
									>
										<Bot class="h-4 w-4" />
									</div>
									<span
										class="rounded-full bg-emerald-100 px-2 py-0.5 text-[11px] font-medium text-emerald-700"
									>
										Active
									</span>
								</div>
								<p class="font-semibold">{name}</p>
								<p class="mt-1 text-xs text-muted-foreground">
									{i % 2 === 0 ? 'General purpose' : 'Specialized'} agent
								</p>
								<button
									class="mt-3 w-full rounded-lg border border-border py-1.5 text-xs font-medium transition-colors hover:bg-muted"
								>
									Configure
								</button>
							</div>
						{/each}
					</div>
				</div>

			<!-- ─ CREATE AGENT ─ -->
			{:else if activeItem === 'agents-create'}
				<div class="space-y-4">
					<h2 class="text-2xl font-bold">Create Agent</h2>
					<div
						class="rounded-xl border border-dashed border-border p-16 text-center text-muted-foreground"
					>
						<Rocket class="mx-auto mb-3 h-10 w-10 opacity-30" />
						<p class="text-sm">Agent builder coming soon</p>
					</div>
				</div>

			<!-- ─ SKILLS ─ -->
			{:else if activeItem === 'skills' || activeItem === 'skills-all'}
				<div class="space-y-4">
					<h2 class="text-2xl font-bold">Skills Library</h2>
					<p class="text-muted-foreground">Browse, deploy, and manage AI skills.</p>
					<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
						{#each ['frontend-design', 'doc-coauthoring', 'mcp-builder', 'pdf', 'allium', 'algorithmic-art', 'canvas-design', 'xlsx', 'pptx', 'docx', 'intent-layer', 'internal-comms'] as skill}
							<div
								class="flex items-center gap-3 rounded-xl border border-border bg-card p-3 shadow-sm"
							>
								<div
									class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-purple-100 text-purple-600"
								>
									<Puzzle class="h-4 w-4" />
								</div>
								<div class="min-w-0 flex-1">
									<p class="truncate text-sm font-medium">{skill}</p>
									<p class="text-xs text-muted-foreground">Skill · Active</p>
								</div>
								<span
									class="shrink-0 rounded-full bg-emerald-100 px-2 py-0.5 text-[10px] font-medium text-emerald-700"
								>
									✓
								</span>
							</div>
						{/each}
					</div>
				</div>

			<!-- ─ AI APPS ─ -->
			{:else if activeItem === 'ai-apps' || activeItem.startsWith('apps-')}
				{@const appMap: Record<
					string,
					{ title: string; desc: string; icon: any; color: string; examples: string[] }
				> = {
					healthcare: {
						title: 'Healthcare AI',
						desc: 'AI solutions for clinical documentation, patient data analysis, and medical record processing.',
						icon: Stethoscope,
						color: 'text-rose-500',
						examples: ['Clinical Notes', 'Patient Summary', 'Lab Report Analysis', 'ICD Coding']
					},
					finance: {
						title: 'Finance AI',
						desc: 'Intelligent tools for financial analysis, reporting, fraud detection, and compliance.',
						icon: BarChart3,
						color: 'text-emerald-500',
						examples: ['Risk Analysis', 'Report Generator', 'Fraud Detection', 'Tax Automation']
					},
					legal: {
						title: 'Legal AI',
						desc: 'Contract review, legal research, document drafting, and compliance monitoring.',
						icon: Scale,
						color: 'text-amber-500',
						examples: ['Contract Review', 'Legal Research', 'Doc Drafting', 'Compliance Check']
					}
				}}
				{@const vertical = activeItem.replace('apps-', '') || 'healthcare'}
				{@const app = appMap[vertical] ?? appMap.healthcare}
				{@const AppIcon = app.icon}
				<div class="space-y-5">
					<div class="flex items-center gap-3">
						<div class="flex h-12 w-12 items-center justify-center rounded-xl bg-muted">
							<AppIcon class="h-6 w-6 {app.color}" />
						</div>
						<div>
							<h2 class="text-2xl font-bold">{app.title}</h2>
							<p class="text-sm text-muted-foreground">{app.desc}</p>
						</div>
					</div>
					<div class="grid gap-3 sm:grid-cols-2">
						{#each app.examples as ex}
							<div class="rounded-xl border border-border bg-card p-4 shadow-sm">
								<p class="font-semibold">{ex}</p>
								<p class="mt-1 text-xs text-muted-foreground">AI-powered module</p>
								<button
									class="mt-3 rounded-lg bg-primary px-3 py-1.5 text-xs text-primary-foreground transition-opacity hover:opacity-90"
								>
									Open App
								</button>
							</div>
						{/each}
					</div>
				</div>

			<!-- ─ CODING ASSISTANTS ─ -->
			{:else if activeItem === 'coding' || activeItem.startsWith('coding-')}
				{@const focused = codingTools.find((t) => t.id === activeItem)}
				{#if focused && activeItem !== 'coding'}
					<!-- Single tool detail -->
					<div class="space-y-4">
						<h2 class="text-2xl font-bold">{focused.label}</h2>
						<p class="text-muted-foreground">{focused.desc}</p>
						<div class="rounded-xl border border-border bg-card p-6 shadow-sm">
							<div class="flex items-center gap-3">
								<div class="flex h-12 w-12 items-center justify-center rounded-xl bg-blue-100">
									<Code2 class="h-6 w-6 text-blue-600" />
								</div>
								<div>
									<span class="rounded-full px-2 py-0.5 text-xs font-medium {focused.badgeColor}">
										{focused.badge}
									</span>
									<p class="mt-1 text-sm text-muted-foreground">Ready to use</p>
								</div>
							</div>
							<button
								class="mt-5 rounded-lg bg-primary px-5 py-2 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90"
							>
								Launch {focused.label}
							</button>
						</div>
					</div>
				{:else}
					<!-- All coding tools -->
					<div class="space-y-4">
						<h2 class="text-2xl font-bold">Coding Assistants</h2>
						<p class="text-muted-foreground">AI-powered coding tools to accelerate development.</p>
						<div class="grid gap-4 sm:grid-cols-2">
							{#each codingTools as tool (tool.id)}
								<button
									class="rounded-xl border border-border bg-card p-5 text-left shadow-sm transition-all hover:-translate-y-0.5 hover:shadow-md"
									onclick={() => selectItem(tool.id)}
								>
									<div class="mb-3 flex items-start justify-between">
										<div
											class="flex h-9 w-9 items-center justify-center rounded-lg bg-blue-100 text-blue-600"
										>
											<Code2 class="h-5 w-5" />
										</div>
										<span class="rounded-full px-2 py-0.5 text-[11px] font-medium {tool.badgeColor}">
											{tool.badge}
										</span>
									</div>
									<p class="font-semibold">{tool.label}</p>
									<p class="mt-1 text-xs leading-relaxed text-muted-foreground">{tool.desc}</p>
								</button>
							{/each}
						</div>
					</div>
				{/if}

			<!-- ─ PERSONAL ASSISTANT ─ -->
			{:else if activeItem === 'personal' || activeItem === 'personal-openclaw'}
				<div class="space-y-5">
					<h2 class="text-2xl font-bold">Personal Assistant</h2>
					<p class="text-muted-foreground">Your personal AI companion, running on your own devices.</p>
					<div class="rounded-xl border border-border bg-card p-6 shadow-sm">
						<div class="flex items-center gap-4">
							<div
								class="flex h-16 w-16 items-center justify-center rounded-full bg-gradient-to-br from-purple-400 to-indigo-500 shadow-md"
							>
								<User2 class="h-8 w-8 text-white" />
							</div>
							<div>
								<h3 class="text-xl font-bold">OpenClaw</h3>
								<p class="text-sm text-muted-foreground">
									A personal digital assistant you run on your own devices
								</p>
								<div class="mt-1 flex items-center gap-1.5">
									<span class="h-1.5 w-1.5 rounded-full bg-emerald-400"></span>
									<span class="text-xs text-emerald-600">Active</span>
								</div>
							</div>
						</div>
						<div class="mt-6 grid grid-cols-3 gap-4">
							{#each ['Daily Digest', 'Task Manager', 'Memory Bank'] as feature}
								<div class="rounded-lg bg-muted p-3 text-center">
									<p class="text-sm font-medium">{feature}</p>
									<p class="mt-0.5 text-xs text-muted-foreground">Available</p>
								</div>
							{/each}
						</div>
						<button
							class="mt-5 rounded-lg bg-primary px-5 py-2 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90"
						>
							Open OpenClaw
						</button>
					</div>
				</div>

			<!-- ─ SETTINGS ─ -->
			{:else if activeItem === 'settings'}
				<div class="space-y-4">
					<h2 class="text-2xl font-bold">Settings</h2>
					<p class="text-muted-foreground">Configure your AI Assistant preferences and integrations.</p>
					<div class="space-y-3">
						{#each ['API Keys', 'Agent Defaults', 'Notification Preferences', 'Appearance', 'Privacy & Data'] as section}
							<div
								class="flex items-center justify-between rounded-xl border border-border bg-card px-5 py-4 shadow-sm"
							>
								<div class="flex items-center gap-3">
									<Settings class="h-4 w-4 text-muted-foreground" />
									<p class="font-medium">{section}</p>
								</div>
								<ChevronRight class="h-4 w-4 text-muted-foreground" />
							</div>
						{/each}
					</div>
				</div>

			<!-- ─ FALLBACK ─ -->
			{:else}
				<div class="flex h-full items-center justify-center">
					<div class="text-center text-muted-foreground">
						<Layers class="mx-auto mb-3 h-10 w-10 opacity-30" />
						<p class="text-sm">Select a menu item to continue</p>
					</div>
				</div>
			{/if}
		</main>

		<!-- ── RIGHT PANEL: Contextual info ──────────────────────────────────── -->
		<aside
			class="flex w-64 shrink-0 flex-col overflow-y-auto border-l border-border bg-sidebar xl:w-72"
		>
			<div class="flex-1 space-y-5 p-4">
				<!-- System Status -->
				<section>
					<h3 class="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
						System Status
					</h3>
					<div class="space-y-1.5">
						{#each systemServices as svc (svc.label)}
							<div class="flex items-center justify-between rounded-lg bg-muted/50 px-3 py-2">
								<span class="text-xs font-medium">{svc.label}</span>
								<div class="flex items-center gap-1.5">
									<span class="h-1.5 w-1.5 rounded-full {svc.dot}"></span>
									<span class="text-xs text-muted-foreground">{svc.status}</span>
								</div>
							</div>
						{/each}
					</div>
				</section>

				<!-- Quick stats -->
				<section class="border-t border-border pt-4">
					<h3 class="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
						Usage Today
					</h3>
					<div class="space-y-2">
						{#each [['API calls', '1,842'], ['Tokens used', '2.4 M'], ['Active sessions', '3'], ['Tasks completed', '28']] as [label, val]}
							<div class="flex justify-between text-xs">
								<span class="text-muted-foreground">{label}</span>
								<span class="font-semibold">{val}</span>
							</div>
						{/each}
					</div>
				</section>

				<!-- Trending skills -->
				<section class="border-t border-border pt-4">
					<h3 class="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
						Trending Skills
					</h3>
					<div class="space-y-1">
						{#each trendingSkills as skill (skill)}
							<button
								class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-xs transition-colors hover:bg-sidebar-accent"
								onclick={() => selectItem('skills-all')}
							>
								<Star class="h-3 w-3 shrink-0 text-amber-400" />
								<span class="truncate">{skill}</span>
							</button>
						{/each}
					</div>
				</section>

				<!-- Tip -->
				<section class="border-t border-border pt-4">
					<h3 class="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
						Pro Tip
					</h3>
					<div class="rounded-lg border border-blue-200 bg-blue-50 p-3">
						<p class="text-xs leading-relaxed text-blue-700">
							<strong>Did you know?</strong> You can chain Skills together to build powerful multi-step
							AI workflows without writing any code.
						</p>
					</div>
				</section>

				<!-- Latest updates -->
				<section class="border-t border-border pt-4">
					<h3 class="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
						Latest Updates
					</h3>
					<div class="space-y-2">
						{#each [{ label: 'Claude Code v1.2', note: 'New hooks system' }, { label: 'OpenClaw 0.9', note: 'Memory bank released' }, { label: 'Skills v3', note: 'allium language support' }] as upd (upd.label)}
							<div class="rounded-md border border-border bg-card px-3 py-2">
								<p class="text-xs font-medium">{upd.label}</p>
								<p class="text-[11px] text-muted-foreground">{upd.note}</p>
							</div>
						{/each}
					</div>
				</section>
			</div>
		</aside>
	</div>

	<!-- ╔══════════════════════════════════════════════════════════════════════
	     BOTTOM PANEL — Footer
	     ══════════════════════════════════════════════════════════════════════╗ -->
	<footer
		class="flex shrink-0 items-center justify-between border-t border-border bg-card px-6 py-2.5 text-xs text-muted-foreground"
	>
		<div class="flex items-center gap-4">
			<span>© 2026 My AI Assistant</span>
			<a href="##" class="transition-colors hover:text-foreground">Privacy</a>
			<a href="##" class="transition-colors hover:text-foreground">Terms</a>
			<a href="##" class="transition-colors hover:text-foreground">Docs</a>
			<a href="##" class="transition-colors hover:text-foreground">GitHub</a>
		</div>
		<div class="flex items-center gap-2">
			<Activity class="h-3 w-3 text-emerald-500" />
			<span>All systems operational</span>
			<span class="opacity-30">·</span>
			<span>v1.0.0</span>
		</div>
	</footer>
</div>

<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import * as Breadcrumb from '$lib/components/ui/breadcrumb/index.js';
	import { Separator } from '$lib/components/ui/separator/index.js';
	import * as Sidebar from '$lib/components/ui/sidebar/index.js';
	import DspyStudio from '$lib/components/shared-ui/dspy/DspyStudio.svelte';

	type MenuItem = {
		id: string;
		title: string;
		type: 'item';
		icon: string;
	};

	type NullMenuItem = {
		id: string;
		title: string;
		type: 'null';
	};

	const null_menu_item: NullMenuItem = {
		id: 'null',
		title: 'null',
		type: 'null'
	};

	// Menu items for AI Assistant
	const menuItems: MenuItem[] = [
		{ id: 'dashboard', title: 'Dashboard', type: 'item', icon: 'layout-dashboard' },
		{ id: 'agents', title: 'Agents', type: 'item', icon: 'bot' },
		{ id: 'skills', title: 'Skills', type: 'item', icon: 'sparkles' },
		{ id: 'ai-apps', title: 'AI Apps', type: 'item', icon: 'app-window' },
		{ id: 'dspy', title: 'DSPy Studio', type: 'item', icon: 'dspy' },
		{ id: 'coding', title: 'Coding Assistant', type: 'item', icon: 'code-2' },
		{ id: 'personal', title: 'Personal Assistant', type: 'item', icon: 'user' },
		{ id: 'knowledge', title: 'Knowledge Base', type: 'item', icon: 'book-open' },
		{ id: 'settings', title: 'Settings', type: 'item', icon: 'settings' }
	];

	let activeView = $state<MenuItem | NullMenuItem>(null_menu_item);
	let sidebarOpen = $state(true);

	onMount(() => {
		// Set default view to dashboard
		activeView = menuItems[0];
	});

	function handleMenuSelect(item: MenuItem) {
		console.log('Menu selected:', item.title);
		activeView = item;
	}

	function getIcon(iconName: string) {
		switch (iconName) {
			case 'layout-dashboard': return '📊';
			case 'bot': return '🤖';
			case 'sparkles': return '✨';
			case 'app-window': return '📱';
			case 'dspy': return '🧬';
			case 'code-2': return '💻';
			case 'user': return '👤';
			case 'book-open': return '📖';
			case 'settings': return '⚙️';
			default: return '📄';
		}
	}
</script>

<Sidebar.Provider>
	<div class="flex h-screen w-full flex-col">
		<!-- Top Panel: Header with Logo, Banner, Slogans -->
		<header class="flex h-20 shrink-0 items-center justify-between border-b bg-gradient-to-r from-blue-600 via-purple-600 to-indigo-600 px-6 text-white shadow-lg">
			<div class="flex items-center gap-4">
				<!-- Logo -->
				<div class="flex h-12 w-12 items-center justify-center rounded-xl bg-white/20 backdrop-blur-sm">
					<span class="text-2xl">🤖</span>
				</div>
				<div>
					<!-- App Name -->
					<h1 class="text-xl font-bold">My AI Assistant</h1>
					<!-- Slogans -->
					<p class="text-xs text-blue-100">Your Comprehensive AI Productivity Hub</p>
				</div>
			</div>

			<!-- Banner Image / Hero Section -->
			<div class="hidden items-center gap-6 md:flex">
				<div class="rounded-lg bg-white/10 px-4 py-2 backdrop-blur-sm">
					<span class="text-sm">🚀 Empowering Your Workflow with AI</span>
				</div>
				<div class="rounded-lg bg-white/10 px-4 py-2 backdrop-blur-sm">
					<span class="text-sm">⚡ Agents • Skills • Apps • Assistants</span>
				</div>
			</div>

			<!-- User Actions -->
			<div class="flex items-center gap-3">
				<button class="rounded-lg bg-white/20 px-4 py-2 text-sm font-medium transition-colors hover:bg-white/30">
					Help
				</button>
				<div class="flex h-10 w-10 items-center justify-center rounded-full bg-white/20">
					<span class="text-sm font-bold">U</span>
				</div>
			</div>
		</header>

		<!-- Main Content Area with Left, Middle, Right Panels -->
		<div class="flex flex-1 overflow-hidden">
			<!-- Left Panel: Menu System -->
			<aside class="flex w-64 shrink-0 flex-col border-r bg-muted/30">
				<nav class="flex-1 space-y-1 overflow-y-auto p-4">
					{#each menuItems as item}
						<button
							class="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors {activeView.id === item.id ? 'bg-primary text-primary-foreground shadow-sm' : 'hover:bg-muted'}"
							onclick={() => handleMenuSelect(item)}
						>
							<span class="text-lg">{getIcon(item.icon)}</span>
							{item.title}
						</button>
					{/each}
				</nav>
			</aside>

			<!-- Middle Panel: Selected Page Content -->
			<main class="flex-1 overflow-y-auto bg-background p-6">
				<!-- Breadcrumb -->
				<div class="mb-6 flex items-center gap-2 text-sm text-muted-foreground">
					<span>Home</span>
					<span>/</span>
					<span class="font-medium text-foreground">{activeView.title}</span>
				</div>

				<!-- Page Content -->
				{#if activeView.id === 'dspy'}
					<div class="-m-6 h-[calc(100%+3rem)]">
						<DspyStudio />
					</div>
				{:else if activeView.id === 'dashboard'}
					<div class="space-y-6">
						<!-- Welcome Section -->
						<div class="rounded-xl bg-gradient-to-r from-blue-500 to-purple-600 p-6 text-white shadow-lg">
							<h2 class="text-2xl font-bold">Welcome to Your AI Assistant Hub</h2>
							<p class="mt-2 text-blue-100">Your comprehensive platform for AI-powered productivity</p>
						</div>

						<!-- Quick Stats -->
						<div class="grid gap-4 md:grid-cols-4">
							<div class="rounded-lg border bg-card p-4 shadow-sm">
								<div class="text-sm font-medium text-muted-foreground">Active Agents</div>
								<div class="text-2xl font-bold">12</div>
								<div class="text-xs text-green-600">+2 this week</div>
							</div>
							<div class="rounded-lg border bg-card p-4 shadow-sm">
								<div class="text-sm font-medium text-muted-foreground">Skills Available</div>
								<div class="text-2xl font-bold">48</div>
								<div class="text-xs text-green-600">+5 this week</div>
							</div>
							<div class="rounded-lg border bg-card p-4 shadow-sm">
								<div class="text-sm font-medium text-muted-foreground">AI Apps</div>
								<div class="text-2xl font-bold">24</div>
								<div class="text-xs text-blue-600">8 vertical markets</div>
							</div>
							<div class="rounded-lg border bg-card p-4 shadow-sm">
								<div class="text-sm font-medium text-muted-foreground">Tasks Completed</div>
								<div class="text-2xl font-bold">1,284</div>
								<div class="text-xs text-green-600">+127 today</div>
							</div>
						</div>

						<!-- Featured AI Tools -->
						<div class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
							<div class="rounded-lg border bg-card p-4 shadow-sm hover:shadow-md transition-shadow">
								<div class="flex items-center gap-3">
									<div class="flex h-10 w-10 items-center justify-center rounded-lg bg-blue-100">
										<span class="text-blue-600">🤖</span>
									</div>
									<div>
										<h3 class="font-semibold">Claude Code</h3>
										<p class="text-sm text-muted-foreground">Advanced coding assistant</p>
									</div>
								</div>
							</div>
							<div class="rounded-lg border bg-card p-4 shadow-sm hover:shadow-md transition-shadow">
								<div class="flex items-center gap-3">
									<div class="flex h-10 w-10 items-center justify-center rounded-lg bg-purple-100">
										<span class="text-purple-600">💻</span>
									</div>
									<div>
										<h3 class="font-semibold">Qwen Code</h3>
										<p class="text-sm text-muted-foreground">Alibaba's AI coding agent</p>
									</div>
								</div>
							</div>
							<div class="rounded-lg border bg-card p-4 shadow-sm hover:shadow-md transition-shadow">
								<div class="flex items-center gap-3">
									<div class="flex h-10 w-10 items-center justify-center rounded-lg bg-green-100">
										<span class="text-green-600">⚡</span>
									</div>
									<div>
										<h3 class="font-semibold">OpenCode</h3>
										<p class="text-sm text-muted-foreground">Open-source coding agent</p>
									</div>
								</div>
							</div>
							<div class="rounded-lg border bg-card p-4 shadow-sm hover:shadow-md transition-shadow">
								<div class="flex items-center gap-3">
									<div class="flex h-10 w-10 items-center justify-center rounded-lg bg-orange-100">
										<span class="text-orange-600">🧠</span>
									</div>
									<div>
										<h3 class="font-semibold">OpenClaw</h3>
										<p class="text-sm text-muted-foreground">Personal digital assistant</p>
									</div>
								</div>
							</div>
							<div class="rounded-lg border bg-card p-4 shadow-sm hover:shadow-md transition-shadow">
								<div class="flex items-center gap-3">
									<div class="flex h-10 w-10 items-center justify-center rounded-lg bg-red-100">
										<span class="text-red-600">📊</span>
									</div>
									<div>
										<h3 class="font-semibold">Codex</h3>
										<p class="text-sm text-muted-foreground">OpenAI code generator</p>
									</div>
								</div>
							</div>
							<div class="rounded-lg border bg-card p-4 shadow-sm hover:shadow-md transition-shadow">
								<div class="flex items-center gap-3">
									<div class="flex h-10 w-10 items-center justify-center rounded-lg bg-cyan-100">
										<span class="text-cyan-600">🚀</span>
									</div>
									<div>
										<h3 class="font-semibold">Crush</h3>
										<p class="text-sm text-muted-foreground">Go-based coding assistant</p>
									</div>
								</div>
							</div>
						</div>

						<!-- Recent Activity -->
						<div class="rounded-lg border bg-card p-4 shadow-sm">
							<h3 class="mb-4 text-lg font-semibold">Recent Activity</h3>
							<div class="space-y-3">
								<div class="flex items-center justify-between">
									<div class="flex items-center gap-3">
										<div class="h-2 w-2 rounded-full bg-green-500"></div>
										<span class="text-sm">Completed code review with Claude Code</span>
									</div>
									<span class="text-xs text-muted-foreground">2 min ago</span>
								</div>
								<div class="flex items-center justify-between">
									<div class="flex items-center gap-3">
										<div class="h-2 w-2 rounded-full bg-blue-500"></div>
										<span class="text-sm">Deployed new skill to production</span>
									</div>
									<span class="text-xs text-muted-foreground">15 min ago</span>
								</div>
								<div class="flex items-center justify-between">
									<div class="flex items-center gap-3">
										<div class="h-2 w-2 rounded-full bg-purple-500"></div>
										<span class="text-sm">Configured OpenClaw preferences</span>
									</div>
									<span class="text-xs text-muted-foreground">1 hour ago</span>
								</div>
								<div class="flex items-center justify-between">
									<div class="flex items-center gap-3">
										<div class="h-2 w-2 rounded-full bg-orange-500"></div>
										<span class="text-sm">Added new AI app to marketplace</span>
									</div>
									<span class="text-xs text-muted-foreground">3 hours ago</span>
								</div>
							</div>
						</div>
					</div>
				{:else}
					<div class="space-y-6">
						<h2 class="text-2xl font-bold">{activeView.title}</h2>
						<p class="text-muted-foreground">
							{#if activeView.id === 'agents'}Manage and configure your AI agents for various tasks
							{:else if activeView.id === 'skills'}Browse and activate specialized AI skills
							{:else if activeView.id === 'ai-apps'}Discover AI applications for vertical markets
							{:else if activeView.id === 'coding'}Access your coding assistants: Claude Code, Qwen Code, OpenCode, Codex, Crush
							{:else if activeView.id === 'personal'}Your personalized AI assistant (OpenClaw)
							{:else if activeView.id === 'knowledge'}Access your organized knowledge and documentation
							{:else if activeView.id === 'settings'}Configure your AI Assistant preferences{/if}
						</p>
						<div class="rounded-lg border bg-card p-8 text-center">
							<p class="text-muted-foreground">{activeView.title} interface - Coming Soon</p>
						</div>
					</div>
				{/if}
			</main>

			<!-- Right Panel: Additional Information -->
			<aside class="flex w-72 shrink-0 flex-col border-l bg-muted/30">
				<div class="flex-1 overflow-y-auto p-4">
					<!-- Context Panel -->
					<div class="rounded-lg border bg-card p-4 shadow-sm">
						<h3 class="mb-3 text-sm font-semibold">Quick Actions</h3>
						<div class="space-y-2">
							<button class="w-full rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90">
								+ New Agent
							</button>
							<button class="w-full rounded-md border bg-background px-3 py-2 text-sm font-medium transition-colors hover:bg-muted">
								Browse Skills
							</button>
							<button class="w-full rounded-md border bg-background px-3 py-2 text-sm font-medium transition-colors hover:bg-muted">
								View Documentation
							</button>
						</div>
					</div>

					<!-- Tips Section -->
					<div class="mt-4 rounded-lg border bg-card p-4 shadow-sm">
						<h3 class="mb-3 text-sm font-semibold">💡 Tip of the Day</h3>
						<p class="text-sm text-muted-foreground">
							Combine multiple AI agents to create powerful workflows. Try using Claude Code for code review and OpenClaw for task automation.
						</p>
					</div>

					<!-- Status Section -->
					<div class="mt-4 rounded-lg border bg-card p-4 shadow-sm">
						<h3 class="mb-3 text-sm font-semibold">System Status</h3>
						<div class="space-y-2 text-sm">
							<div class="flex items-center justify-between">
								<span>AI Services</span>
								<span class="flex items-center gap-1 text-green-600">
									<span class="h-2 w-2 rounded-full bg-green-500"></span>
									Online
								</span>
							</div>
							<div class="flex items-center justify-between">
								<span>Knowledge Base</span>
								<span class="flex items-center gap-1 text-green-600">
									<span class="h-2 w-2 rounded-full bg-green-500"></span>
									Online
								</span>
							</div>
							<div class="flex items-center justify-between">
								<span>Agent Hub</span>
								<span class="flex items-center gap-1 text-green-600">
									<span class="h-2 w-2 rounded-full bg-green-500"></span>
									Online
								</span>
							</div>
						</div>
					</div>
				</div>
			</aside>
		</div>

		<!-- Bottom Panel: Footer -->
		<footer class="flex h-12 shrink-0 items-center justify-between border-t bg-muted/50 px-6 text-xs text-muted-foreground">
			<div>
				© 2026 My AI Assistant. All rights reserved.
			</div>
			<div class="flex items-center gap-4">
				<button role="link" class="transition-colors hover:text-foreground">Privacy Policy</button>
				<button role="link" class="transition-colors hover:text-foreground">Terms of Service</button>
				<button role="link" class="transition-colors hover:text-foreground">Documentation</button>
				<button role="link" class="transition-colors hover:text-foreground">Support</button>
			</div>
			<div>
				Version 1.0.0
			</div>
		</footer>
	</div>
</Sidebar.Provider>

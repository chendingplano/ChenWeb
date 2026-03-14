<script lang="ts">
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import * as Avatar from '$lib/components/ui/avatar/index.js';

	import LayoutDashboardIcon from '@lucide/svelte/icons/layout-dashboard';
	import BotIcon from '@lucide/svelte/icons/bot';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import AppWindowIcon from '@lucide/svelte/icons/app-window';
	import CodeIcon from '@lucide/svelte/icons/code-2';
	import UserIcon from '@lucide/svelte/icons/user';
	import BookOpenIcon from '@lucide/svelte/icons/book-open';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import InfoIcon from '@lucide/svelte/icons/info';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import MoreHorizontalIcon from '@lucide/svelte/icons/more-horizontal';
	import UserCircle2Icon from '@lucide/svelte/icons/user-circle-2';
	import CreditCardIcon from '@lucide/svelte/icons/credit-card';
	import LogOutIcon from '@lucide/svelte/icons/log-out';

	export type ActiveSelection = {
		itemId: string;
		childId?: string;
		itemTitle: string;
		childTitle?: string;
	};

	type NavChild = { id: string; title: string };
	type NavItem = {
		id: string;
		title: string;
		icon: any;
		isLeaf?: boolean;
		children?: NavChild[];
	};

	let {
		onMenuSelect,
		activeMenu = null,
		darkMode = true,
		width = 260
	}: {
		onMenuSelect: (s: ActiveSelection) => void;
		activeMenu: ActiveSelection | null;
		darkMode: boolean;
		width?: number;
	} = $props();

	// Track expand/collapse per nav item
	let expanded = $state<Record<string, boolean>>({});

	const mainNav: NavItem[] = [
		{ id: 'dashboard', title: 'Dashboard', icon: LayoutDashboardIcon, isLeaf: true },
		{
			id: 'agents',
			title: 'Agents',
			icon: BotIcon,
			children: [
				{ id: 'agents-my', title: 'My Agents' },
				{ id: 'agents-store', title: 'Agent Store' },
				{ id: 'agents-builder', title: 'Agent Builder' }
			]
		},
		{
			id: 'skills',
			title: 'Skills',
			icon: ZapIcon,
			children: [
				{ id: 'skills-my', title: 'My Skills' },
				{ id: 'skills-store', title: 'Skill Store' },
				{ id: 'skills-create', title: 'Create Skill' }
			]
		},
		{
			id: 'applications',
			title: 'Applications',
			icon: AppWindowIcon,
			children: [
				{ id: 'apps-installed', title: 'Installed Apps' },
				{ id: 'apps-marketplace', title: 'Marketplace' },
				{ id: 'apps-build', title: 'Build App' }
			]
		},
		{
			id: 'coding',
			title: 'Coding Assistant',
			icon: CodeIcon,
			children: [
				{ id: 'coding-review', title: 'Code Review' },
				{ id: 'coding-generate', title: 'Code Generation' },
				{ id: 'coding-debug', title: 'Debug Assistant' },
				{ id: 'coding-refactor', title: 'Refactoring' }
			]
		},
		{
			id: 'personal',
			title: 'Personal Assistant',
			icon: UserIcon,
			children: [
				{ id: 'personal-briefing', title: 'Daily Briefing' },
				{ id: 'personal-tasks', title: 'Tasks & Calendar' },
				{ id: 'personal-email', title: 'Email Assistant' },
				{ id: 'personal-notes', title: 'Meeting Notes' }
			]
		},
		{
			id: 'knowledge',
			title: 'Knowledge Base',
			icon: BookOpenIcon,
			children: [
				{ id: 'kb-documents', title: 'Documents' },
				{ id: 'kb-search', title: 'Search' },
				{ id: 'kb-import', title: 'Import Sources' },
				{ id: 'kb-graph', title: 'Graph View' }
			]
		}
	];

	const bottomNav: NavItem[] = [
		{
			id: 'settings',
			title: 'Settings',
			icon: SettingsIcon,
			children: [
				{ id: 'settings-general', title: 'General' },
				{ id: 'settings-models', title: 'AI Models' },
				{ id: 'settings-integrations', title: 'Integrations' },
				{ id: 'settings-security', title: 'Security' }
			]
		},
		{ id: 'about', title: 'About', icon: InfoIcon, isLeaf: true }
	];

	const user = { name: 'Alex Johnson', email: 'alex@example.com', avatar: '/avatars/user.jpg' };

	function selectItem(item: NavItem, child?: NavChild) {
		onMenuSelect({
			itemId: item.id,
			childId: child?.id,
			itemTitle: item.title,
			childTitle: child?.title
		});
	}

	function toggleExpand(id: string) {
		expanded[id] = !expanded[id];
	}

	function isActive(item: NavItem, child?: NavChild): boolean {
		if (!activeMenu) return false;
		if (child) return activeMenu.childId === child.id;
		return activeMenu.itemId === item.id && !activeMenu.childId;
	}

	function isParentActive(item: NavItem): boolean {
		return !!activeMenu && activeMenu.itemId === item.id;
	}

	// Surface colors
	let bg = $derived(darkMode ? '#0F172A' : '#ffffff');
	let borderColor = $derived(darkMode ? '#1E293B' : '#e2e8f0');
	let textMuted = $derived(darkMode ? '#64748b' : '#94a3b8');
	let textNormal = $derived(darkMode ? '#94a3b8' : '#475569');
	let textActive = $derived(darkMode ? '#F8FAFC' : '#0f172a');
	let hoverBg = $derived(darkMode ? 'rgba(30,41,59,0.8)' : 'rgba(241,245,249,0.9)');
	let activeBg = $derived(
		darkMode ? 'rgba(34,197,94,0.12)' : 'rgba(34,197,94,0.1)'
	);
	let subBg = $derived(darkMode ? 'rgba(15,23,42,0.5)' : 'rgba(248,250,252,0.8)');
</script>

<aside
	class="flex-shrink-0 flex flex-col overflow-hidden"
	style="width:{width}px; background:{bg}; border-right:1px solid {borderColor};"
>
	<!-- Sidebar label -->
	<div class="px-4 pt-4 pb-2">
		<span class="text-xs font-semibold uppercase tracking-widest" style="color:{textMuted}; font-family:'Fira Code',monospace;">
			Navigation
		</span>
	</div>

	<!-- Main nav (scrollable) -->
	<nav class="flex-1 overflow-y-auto px-2 pb-2" style="scrollbar-width:thin; scrollbar-color:{borderColor} transparent;">
		{#each mainNav as item (item.id)}
			<div class="mb-0.5">
				{#if item.isLeaf}
					<!-- Leaf item -->
					<button
						onclick={() => selectItem(item)}
						class="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-all duration-150 cursor-pointer"
						style="background:{isActive(item) ? activeBg : 'transparent'}; color:{isActive(item) ? '#22C55E' : textNormal}; border: 1px solid {isActive(item) ? 'rgba(34,197,94,0.25)' : 'transparent'};"
						onmouseenter={(e) => { if (!isActive(item)) (e.currentTarget as HTMLElement).style.background = hoverBg; (e.currentTarget as HTMLElement).style.color = textActive; }}
						onmouseleave={(e) => { if (!isActive(item)) { (e.currentTarget as HTMLElement).style.background = 'transparent'; (e.currentTarget as HTMLElement).style.color = textNormal; } }}
					>
						<item.icon class="w-4 h-4 flex-shrink-0" />
						<span class="flex-1 text-left font-medium">{item.title}</span>
					</button>
				{:else}
					<!-- Expandable item -->
					<button
						onclick={() => toggleExpand(item.id)}
						class="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-all duration-150 cursor-pointer"
						style="background:{isParentActive(item) && !activeMenu?.childId ? activeBg : 'transparent'}; color:{isParentActive(item) ? '#22C55E' : textNormal};"
						onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = hoverBg; (e.currentTarget as HTMLElement).style.color = textActive; }}
						onmouseleave={(e) => { if (!isParentActive(item)) { (e.currentTarget as HTMLElement).style.background = 'transparent'; (e.currentTarget as HTMLElement).style.color = textNormal; } else { (e.currentTarget as HTMLElement).style.background = isParentActive(item) && !activeMenu?.childId ? activeBg : 'transparent'; } }}
					>
						<item.icon class="w-4 h-4 flex-shrink-0" />
						<span class="flex-1 text-left font-medium">{item.title}</span>
						<ChevronRightIcon
							class="w-3.5 h-3.5 flex-shrink-0 transition-transform duration-200"
							style="transform: rotate({expanded[item.id] ? '90deg' : '0deg'});"
						/>
					</button>

					<!-- Sub-items -->
					{#if expanded[item.id]}
						<div
							class="ml-3 mt-0.5 mb-1 rounded-lg overflow-hidden"
							style="background:{subBg}; border-left: 2px solid {borderColor};"
						>
							{#each item.children ?? [] as child (child.id)}
								<button
									onclick={() => selectItem(item, child)}
									class="flex w-full items-center gap-2 px-3 py-2 text-sm transition-all duration-150 cursor-pointer"
									style="color:{isActive(item, child) ? '#22C55E' : textMuted}; background:{isActive(item, child) ? activeBg : 'transparent'};"
									onmouseenter={(e) => { if (!isActive(item, child)) { (e.currentTarget as HTMLElement).style.background = hoverBg; (e.currentTarget as HTMLElement).style.color = textActive; } }}
									onmouseleave={(e) => { if (!isActive(item, child)) { (e.currentTarget as HTMLElement).style.background = 'transparent'; (e.currentTarget as HTMLElement).style.color = textMuted; } }}
								>
									<div
										class="w-1 h-1 rounded-full flex-shrink-0"
										style="background:{isActive(item, child) ? '#22C55E' : 'currentColor'}; opacity:0.6;"
									></div>
									{child.title}
								</button>
							{/each}
						</div>
					{/if}
				{/if}
			</div>
		{/each}

		<!-- Separator -->
		<div class="my-3 mx-3" style="height:1px; background:{borderColor};"></div>

		<!-- Bottom nav items (Settings, About) -->
		{#each bottomNav as item (item.id)}
			<div class="mb-0.5">
				{#if item.isLeaf}
					<button
						onclick={() => selectItem(item)}
						class="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-all duration-150 cursor-pointer"
						style="background:{isActive(item) ? activeBg : 'transparent'}; color:{isActive(item) ? '#22C55E' : textMuted}; border:1px solid {isActive(item) ? 'rgba(34,197,94,0.25)' : 'transparent'};"
						onmouseenter={(e) => { if (!isActive(item)) { (e.currentTarget as HTMLElement).style.background = hoverBg; (e.currentTarget as HTMLElement).style.color = textActive; } }}
						onmouseleave={(e) => { if (!isActive(item)) { (e.currentTarget as HTMLElement).style.background = 'transparent'; (e.currentTarget as HTMLElement).style.color = textMuted; } }}
					>
						<item.icon class="w-4 h-4 flex-shrink-0" />
						<span class="flex-1 text-left font-medium">{item.title}</span>
					</button>
				{:else}
					<button
						onclick={() => toggleExpand(item.id)}
						class="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-all duration-150 cursor-pointer"
						style="color:{textMuted};"
						onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = hoverBg; (e.currentTarget as HTMLElement).style.color = textActive; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = 'transparent'; (e.currentTarget as HTMLElement).style.color = textMuted; }}
					>
						<item.icon class="w-4 h-4 flex-shrink-0" />
						<span class="flex-1 text-left font-medium">{item.title}</span>
						<ChevronRightIcon
							class="w-3.5 h-3.5 flex-shrink-0 transition-transform duration-200"
							style="transform: rotate({expanded[item.id] ? '90deg' : '0deg'});"
						/>
					</button>
					{#if expanded[item.id]}
						<div
							class="ml-3 mt-0.5 mb-1 rounded-lg overflow-hidden"
							style="background:{subBg}; border-left: 2px solid {borderColor};"
						>
							{#each item.children ?? [] as child (child.id)}
								<button
									onclick={() => selectItem(item, child)}
									class="flex w-full items-center gap-2 px-3 py-2 text-sm transition-all duration-150 cursor-pointer"
									style="color:{isActive(item, child) ? '#22C55E' : textMuted}; background:{isActive(item, child) ? activeBg : 'transparent'};"
									onmouseenter={(e) => { if (!isActive(item, child)) { (e.currentTarget as HTMLElement).style.background = hoverBg; (e.currentTarget as HTMLElement).style.color = textActive; } }}
									onmouseleave={(e) => { if (!isActive(item, child)) { (e.currentTarget as HTMLElement).style.background = 'transparent'; (e.currentTarget as HTMLElement).style.color = textMuted; } }}
								>
									<div class="w-1 h-1 rounded-full flex-shrink-0" style="background:currentColor;opacity:0.5;"></div>
									{child.title}
								</button>
							{/each}
						</div>
					{/if}
				{/if}
			</div>
		{/each}
	</nav>

	<!-- User section -->
	<div class="flex-shrink-0 p-3" style="border-top:1px solid {borderColor};">
		<div
			class="flex items-center gap-3 rounded-lg p-2"
			style="background:{hoverBg};"
		>
			<!-- Avatar -->
			<Avatar.Root class="size-8 rounded-lg flex-shrink-0">
				<Avatar.Image src={user.avatar} alt={user.name} />
				<Avatar.Fallback
					class="rounded-lg text-xs font-semibold"
					style="background:rgba(34,197,94,0.2); color:#22C55E;"
				>
					{user.name.split(' ').map((n) => n[0]).join('')}
				</Avatar.Fallback>
			</Avatar.Root>

			<!-- Name + email -->
			<div class="flex-1 min-w-0">
				<div class="text-sm font-medium truncate" style="color:{textActive};">{user.name}</div>
				<div class="text-xs truncate" style="color:{textMuted};">{user.email}</div>
			</div>

			<!-- Three-dots dropdown -->
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<button
							{...props}
							class="flex items-center justify-center w-7 h-7 rounded-md transition-colors duration-150 cursor-pointer flex-shrink-0"
							style="color:{textMuted};"
							onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = borderColor; (e.currentTarget as HTMLElement).style.color = textActive; }}
							onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = 'transparent'; (e.currentTarget as HTMLElement).style.color = textMuted; }}
							aria-label="User menu"
						>
							<MoreHorizontalIcon class="w-4 h-4" />
						</button>
					{/snippet}
				</DropdownMenu.Trigger>

				<DropdownMenu.Content
					class="min-w-48 rounded-lg"
					side="top"
					align="end"
					sideOffset={8}
				>
					<DropdownMenu.Label class="p-0 font-normal">
						<div class="flex items-center gap-2 px-2 py-2">
							<Avatar.Root class="size-8 rounded-lg">
								<Avatar.Image src={user.avatar} alt={user.name} />
								<Avatar.Fallback class="rounded-lg text-xs font-semibold" style="background:rgba(34,197,94,0.2);color:#22C55E;">
									{user.name.split(' ').map((n) => n[0]).join('')}
								</Avatar.Fallback>
							</Avatar.Root>
							<div>
								<div class="text-sm font-medium">{user.name}</div>
								<div class="text-xs text-muted-foreground">{user.email}</div>
							</div>
						</div>
					</DropdownMenu.Label>
					<DropdownMenu.Separator />
					<DropdownMenu.Item
						onclick={() => onMenuSelect({ itemId: '__user_info__', itemTitle: 'User Info' })}
					>
						<UserCircle2Icon class="w-4 h-4 mr-2" />
						User Info
					</DropdownMenu.Item>
					<DropdownMenu.Item
						onclick={() => onMenuSelect({ itemId: '__account__', itemTitle: 'Account' })}
					>
						<CreditCardIcon class="w-4 h-4 mr-2" />
						Account
					</DropdownMenu.Item>
					<DropdownMenu.Separator />
					<DropdownMenu.Item
						onclick={() => onMenuSelect({ itemId: '__logout__', itemTitle: 'Logout' })}
					>
						<LogOutIcon class="w-4 h-4 mr-2" />
						Log Out
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		</div>
	</div>
</aside>

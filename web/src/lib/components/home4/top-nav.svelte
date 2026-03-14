<script lang="ts">
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import DropdownMenuPanel from './dropdown-menu-panel.svelte';

	import LayoutDashboardIcon from '@lucide/svelte/icons/layout-dashboard';
	import BotIcon from '@lucide/svelte/icons/bot';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import LayoutGridIcon from '@lucide/svelte/icons/layout-grid';
	import CodeIcon from '@lucide/svelte/icons/code-2';
	import UserIcon from '@lucide/svelte/icons/user';
	import BookOpenIcon from '@lucide/svelte/icons/book-open';
	import UsersIcon from '@lucide/svelte/icons/users';
	import SearchIcon from '@lucide/svelte/icons/search';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import PackageIcon from '@lucide/svelte/icons/package';
	import GitPullRequestIcon from '@lucide/svelte/icons/git-pull-request';
	import BugIcon from '@lucide/svelte/icons/bug';
	import CheckSquareIcon from '@lucide/svelte/icons/square-check';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import MailIcon from '@lucide/svelte/icons/mail';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import UploadIcon from '@lucide/svelte/icons/upload';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import InfoIcon from '@lucide/svelte/icons/info';
	import PanelRightIcon from '@lucide/svelte/icons/panel-right';
	import UserCircle2Icon from '@lucide/svelte/icons/user-circle-2';
	import CreditCardIcon from '@lucide/svelte/icons/credit-card';
	import LogOutIcon from '@lucide/svelte/icons/log-out';

	type ActiveSelection = {
		itemId: string;
		childId?: string;
		itemTitle: string;
		childTitle?: string;
	};

	type NavChild = { id: string; label: string; icon: any };
	type NavItem = { id: string; label: string; icon: any; children: NavChild[] };

	let {
		darkMode,
		activeMenu,
		drawerOpen,
		onSelect,
		onToggleDrawer,
		onOpenCommandPalette
	}: {
		darkMode: boolean;
		activeMenu: ActiveSelection | null;
		drawerOpen: boolean;
		onSelect: (sel: ActiveSelection) => void;
		onToggleDrawer: () => void;
		onOpenCommandPalette: () => void;
	} = $props();

	// --- Timing constants ---
	const NAV_BAR_HEIGHT      = 44;  // nav bar height
	const DROPDOWN_SHOW_DELAY = 100; // ms before dropdown shows on hover
	const DROPDOWN_HIDE_DELAY = 150; // ms before dropdown hides after leave

	// Suppress unused constant warnings
	void NAV_BAR_HEIGHT;

	// --- Colour tokens (all $derived from darkMode) ---
	let navBarBg    = $derived(darkMode ? '#1A1E2C' : '#ECEEF2');       // top nav bar background
	let border      = $derived(darkMode ? '#2D3348' : '#E4E6EB');       // border / divider lines
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');       // primary text
	let textMuted   = $derived(darkMode ? '#64748b' : '#94a3b8');       // muted text
	let accent      = $derived(darkMode ? '#818CF8' : '#6366F1');       // primary accent (indigo)
	let accentTint  = $derived(darkMode ? 'rgba(129,140,248,0.10)' : 'rgba(99,102,241,0.08)'); // accent tint bg
	let hoverBg     = $derived(darkMode ? 'rgba(129,140,248,0.08)' : 'rgba(99,102,241,0.06)'); // item hover bg
	let dividerBg   = $derived(darkMode ? '#2D3348' : '#D0D4DF');       // vertical divider

	// Nav item data structure
	const mainNav: NavItem[] = [
		{
			id: 'dashboard', label: 'Dashboard', icon: LayoutDashboardIcon,
			children: []
		},
		{
			id: 'agents', label: 'Agents', icon: BotIcon,
			children: [
				{ id: 'agents-my', label: 'My Agents', icon: UsersIcon },
				{ id: 'agents-browse', label: 'Browse Library', icon: SearchIcon },
				{ id: 'agents-create', label: 'Create Agent', icon: PlusIcon }
			]
		},
		{
			id: 'skills', label: 'Skills', icon: ZapIcon,
			children: [
				{ id: 'skills-all', label: 'All Skills', icon: ZapIcon },
				{ id: 'skills-active', label: 'Active', icon: ActivityIcon },
				{ id: 'skills-create', label: 'New Skill', icon: PlusIcon }
			]
		},
		{
			id: 'applications', label: 'Applications', icon: LayoutGridIcon,
			children: [
				{ id: 'apps-installed', label: 'Installed', icon: PackageIcon },
				{ id: 'apps-browse', label: 'Browse', icon: SearchIcon },
				{ id: 'apps-configure', label: 'Configure', icon: SettingsIcon }
			]
		},
		{
			id: 'coding', label: 'Coding Assistant', icon: CodeIcon,
			children: [
				{ id: 'coding-review', label: 'Code Review', icon: GitPullRequestIcon },
				{ id: 'coding-gen', label: 'Generation', icon: ZapIcon },
				{ id: 'coding-debug', label: 'Debugger', icon: BugIcon }
			]
		},
		{
			id: 'personal', label: 'Personal Assistant', icon: UserIcon,
			children: [
				{ id: 'personal-tasks', label: 'Tasks', icon: CheckSquareIcon },
				{ id: 'personal-calendar', label: 'Calendar', icon: CalendarIcon },
				{ id: 'personal-email', label: 'Email', icon: MailIcon }
			]
		},
		{
			id: 'knowledge', label: 'Knowledge Base', icon: BookOpenIcon,
			children: [
				{ id: 'kb-docs', label: 'Documents', icon: FileTextIcon },
				{ id: 'kb-search', label: 'Search', icon: SearchIcon },
				{ id: 'kb-import', label: 'Import', icon: UploadIcon }
			]
		}
	];

	// --- Hover dropdown state (local to top-nav) ---
	let openDropdownId = $state<string | null>(null);
	let showTimer      = $state<ReturnType<typeof setTimeout> | null>(null);
	let hideTimer      = $state<ReturnType<typeof setTimeout> | null>(null);

	// Positions for dropdown panels — keyed by item id
	let triggerRects = $state<Record<string, { left: number; top: number }>>({});

	function onItemMouseEnter(itemId: string, el: HTMLElement) {
		if (hideTimer) { clearTimeout(hideTimer); hideTimer = null; }
		if (showTimer) { clearTimeout(showTimer); showTimer = null; }
		showTimer = setTimeout(() => {
			const rect = el.getBoundingClientRect();
			triggerRects[itemId] = { left: rect.left, top: rect.bottom };
			openDropdownId = itemId;
		}, DROPDOWN_SHOW_DELAY);
	}

	function onItemMouseLeave() {
		if (showTimer) { clearTimeout(showTimer); showTimer = null; }
		hideTimer = setTimeout(() => {
			openDropdownId = null;
		}, DROPDOWN_HIDE_DELAY);
	}

	function onDropdownPanelMouseEnter() {
		if (hideTimer) { clearTimeout(hideTimer); hideTimer = null; }
	}

	function onDropdownPanelMouseLeave() {
		hideTimer = setTimeout(() => {
			openDropdownId = null;
		}, DROPDOWN_HIDE_DELAY);
	}

	function selectItem(item: NavItem, child?: NavChild) {
		openDropdownId = null;
		onSelect({
			itemId: item.id,
			childId: child?.id,
			itemTitle: item.label,
			childTitle: child?.label
		});
	}

	function isItemActive(item: NavItem): boolean {
		return !!activeMenu && activeMenu.itemId === item.id;
	}

	function handleKeydown(e: KeyboardEvent, item: NavItem) {
		if (openDropdownId === item.id) {
			if (e.key === 'Escape') {
				openDropdownId = null;
				e.preventDefault();
			}
		}
	}

	const user = { name: 'Alex Johnson', email: 'alex@example.com' };
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<nav
	class="flex-shrink-0 flex items-center px-4 gap-1 relative z-30 select-none"
	style="height:44px; background:{navBarBg}; border-bottom:1px solid {border};"
>
	<!-- LEFT: nav items -->
	<div class="flex items-center gap-0.5 flex-1 min-w-0 overflow-hidden">
		{#each mainNav as item (item.id)}
			{@const isLeaf = item.children.length === 0}
			{@const isActive = isItemActive(item)}
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div
				class="relative flex-shrink-0"
				onmouseenter={(e) => { if (!isLeaf) onItemMouseEnter(item.id, e.currentTarget as HTMLElement); }}
				onmouseleave={() => { if (!isLeaf) onItemMouseLeave(); }}
				onkeydown={(e) => handleKeydown(e, item)}
			>
				<button
					onclick={() => selectItem(item)}
					class="flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors duration-100 cursor-pointer"
					style="
						color:{isActive ? accent : textMuted};
						background:{isActive ? accentTint : 'transparent'};
						border-bottom:{isActive ? `2px solid ${accent}` : '2px solid transparent'};
					"
					onmouseenter={(e) => {
						if (!isActive) {
							(e.currentTarget as HTMLElement).style.background = hoverBg;
							(e.currentTarget as HTMLElement).style.color = textPrimary;
						}
					}}
					onmouseleave={(e) => {
						if (!isActive) {
							(e.currentTarget as HTMLElement).style.background = 'transparent';
							(e.currentTarget as HTMLElement).style.color = textMuted;
						}
					}}
					aria-haspopup={!isLeaf}
					aria-expanded={openDropdownId === item.id}
				>
					<span>{item.label}</span>
					{#if !isLeaf}
						<ChevronDownIcon
							class="w-3.5 h-3.5 flex-shrink-0 transition-transform duration-150"
							style="transform:rotate({openDropdownId === item.id ? '180deg' : '0deg'});"
						/>
					{/if}
				</button>
			</div>
		{/each}
	</div>

	<!-- RIGHT: utility nav -->
	<div class="flex items-center gap-1.5 flex-shrink-0 ml-auto">
		<!-- Settings button -->
		<button
			onclick={() => onSelect({ itemId: 'settings', itemTitle: 'Settings' })}
			class="flex items-center justify-center w-8 h-8 rounded-md transition-colors duration-100 cursor-pointer"
			style="color:{textMuted};"
			onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = hoverBg; (e.currentTarget as HTMLElement).style.color = textPrimary; }}
			onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = 'transparent'; (e.currentTarget as HTMLElement).style.color = textMuted; }}
			aria-label="Settings"
		>
			<SettingsIcon class="w-4 h-4" />
		</button>

		<!-- About button -->
		<button
			onclick={() => onSelect({ itemId: 'about', itemTitle: 'About' })}
			class="flex items-center justify-center w-8 h-8 rounded-md transition-colors duration-100 cursor-pointer"
			style="color:{textMuted};"
			onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = hoverBg; (e.currentTarget as HTMLElement).style.color = textPrimary; }}
			onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = 'transparent'; (e.currentTarget as HTMLElement).style.color = textMuted; }}
			aria-label="About"
		>
			<InfoIcon class="w-4 h-4" />
		</button>

		<!-- Divider -->
		<div class="w-px h-5 flex-shrink-0" style="background:{dividerBg};"></div>

		<!-- ⌘K pill button -->
		<button
			onclick={onOpenCommandPalette}
			class="flex items-center gap-1 px-2.5 py-1 rounded-md text-xs transition-colors duration-100 cursor-pointer"
			style="background:transparent; border:1px solid {border}; color:{textMuted}; font-family:'Fira Code',monospace;"
			onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = hoverBg; (e.currentTarget as HTMLElement).style.color = textPrimary; }}
			onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = 'transparent'; (e.currentTarget as HTMLElement).style.color = textMuted; }}
			aria-label="Open command palette"
		>
			⌘K
		</button>

		<!-- User avatar + DropdownMenu -->
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}
					<button
						{...props}
						class="flex items-center justify-center w-8 h-8 rounded-full text-xs font-bold transition-colors duration-100 cursor-pointer flex-shrink-0"
						style="background:{accent}; color:#ffffff;"
						aria-label="User menu"
					>
						{user.name.split(' ').map((n: string) => n[0]).join('')}
					</button>
				{/snippet}
			</DropdownMenu.Trigger>

			<DropdownMenu.Content
				class="min-w-48 rounded-lg"
				side="bottom"
				align="end"
				sideOffset={6}
			>
				<DropdownMenu.Label class="p-0 font-normal">
					<div class="flex items-center gap-2 px-2 py-2">
						<div
							class="w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold flex-shrink-0"
							style="background:{accent}; color:#ffffff;"
						>
							{user.name.split(' ').map((n: string) => n[0]).join('')}
						</div>
						<div>
							<div class="text-sm font-medium">{user.name}</div>
							<div class="text-xs text-muted-foreground">{user.email}</div>
						</div>
					</div>
				</DropdownMenu.Label>
				<DropdownMenu.Separator />
				<DropdownMenu.Item
					onclick={() => onSelect({ itemId: '__user_info__', itemTitle: 'User Info' })}
				>
					<UserCircle2Icon class="w-4 h-4 mr-2" />
					User Info
				</DropdownMenu.Item>
				<DropdownMenu.Item
					onclick={() => onSelect({ itemId: '__account__', itemTitle: 'Account' })}
				>
					<CreditCardIcon class="w-4 h-4 mr-2" />
					Account
				</DropdownMenu.Item>
				<DropdownMenu.Separator />
				<DropdownMenu.Item
					onclick={() => onSelect({ itemId: '__logout__', itemTitle: 'Log Out' })}
				>
					<LogOutIcon class="w-4 h-4 mr-2" />
					Log Out
				</DropdownMenu.Item>
			</DropdownMenu.Content>
		</DropdownMenu.Root>

		<!-- Panel right toggle -->
		<button
			onclick={onToggleDrawer}
			class="flex items-center justify-center w-8 h-8 rounded-md transition-colors duration-100 cursor-pointer"
			style="color:{drawerOpen ? accent : textMuted}; background:{drawerOpen ? accentTint : 'transparent'};"
			onmouseenter={(e) => { if (!drawerOpen) { (e.currentTarget as HTMLElement).style.background = hoverBg; (e.currentTarget as HTMLElement).style.color = textPrimary; } }}
			onmouseleave={(e) => { if (!drawerOpen) { (e.currentTarget as HTMLElement).style.background = 'transparent'; (e.currentTarget as HTMLElement).style.color = textMuted; } }}
			aria-label="Toggle context drawer"
		>
			<PanelRightIcon class="w-4 h-4" />
		</button>
	</div>
</nav>

<!-- Dropdown panels rendered as portal-like fixed overlays -->
{#each mainNav as item (item.id)}
	{#if item.children.length > 0 && openDropdownId === item.id && triggerRects[item.id]}
		<div
			style="position:fixed; left:{triggerRects[item.id].left}px; top:{triggerRects[item.id].top + 2}px; z-index:1000;"
		>
			<DropdownMenuPanel
				{darkMode}
				items={item.children}
				activeChildId={activeMenu?.itemId === item.id ? activeMenu?.childId : undefined}
				onSelect={(childId, childTitle) => selectItem(item, { id: childId, label: childTitle, icon: null })}
				onMouseEnter={onDropdownPanelMouseEnter}
				onMouseLeave={onDropdownPanelMouseLeave}
			/>
		</div>
	{/if}
{/each}

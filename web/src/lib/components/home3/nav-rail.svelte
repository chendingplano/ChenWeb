<script lang="ts">
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';

	import LayoutDashboardIcon from '@lucide/svelte/icons/layout-dashboard';
	import BotIcon             from '@lucide/svelte/icons/bot';
	import ZapIcon             from '@lucide/svelte/icons/zap';
	import LayoutGridIcon      from '@lucide/svelte/icons/layout-grid';
	import CodeIcon            from '@lucide/svelte/icons/code-2';
	import UserIcon            from '@lucide/svelte/icons/user';
	import BookOpenIcon        from '@lucide/svelte/icons/book-open';
	import SettingsIcon        from '@lucide/svelte/icons/settings';
	import InfoIcon            from '@lucide/svelte/icons/info';
	import ChevronDownIcon     from '@lucide/svelte/icons/chevron-down';
	import PanelLeftIcon       from '@lucide/svelte/icons/panel-left';
	import PanelLeftCloseIcon  from '@lucide/svelte/icons/panel-left-close';
	import MoreHorizontalIcon  from '@lucide/svelte/icons/more-horizontal';
	import UserCircle2Icon     from '@lucide/svelte/icons/user-circle-2';
	import CreditCardIcon      from '@lucide/svelte/icons/credit-card';
	import LogOutIcon          from '@lucide/svelte/icons/log-out';
	import WorkflowIcon        from '@lucide/svelte/icons/workflow';

	type ActiveSelection = {
		itemId: string;
		childId?: string;
		itemTitle: string;
		childTitle?: string;
	};

	type NavChild = { id: string; label: string };
	type NavItem  = {
		id: string;
		label: string;
		icon: any; // lucide component
		children?: NavChild[];
	};

	let {
		darkMode       = true,
		activeMenu     = null,
		pinned         = false,
		expanded       = false,
		width          = 240,
		onSelect,
		onTogglePin,
		onWidthDragStart,
		onHoverChange
	}: {
		darkMode:          boolean;
		activeMenu:        ActiveSelection | null;
		pinned:            boolean;
		expanded:          boolean;
		width:             number;
		onSelect:          (sel: ActiveSelection) => void;
		onTogglePin:       () => void;
		onWidthDragStart:  (e: MouseEvent) => void;
		onHoverChange:     (hovered: boolean) => void;
	} = $props();

	// --- Layout constants ---
	const RAIL_WIDTH_COLLAPSED = 56;  // collapsed icon-rail width in px
	const RAIL_TRANSITION = '200ms ease'; // panel slide animation

	// --- Typography ---
	const fontMono = "'Fira Code', 'Cascadia Code', monospace"; // monospace for badges

	// --- Design tokens ---
	let surface2      = $derived(darkMode ? '#252A3A'  : '#ECEEF2');  // rail background
	let borderColor   = $derived(darkMode ? '#2D3348'  : '#E4E6EB');  // border / divider lines
	let accent        = $derived(darkMode ? '#818CF8'  : '#6366F1');  // primary accent
	let accentTint    = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)'); // tint
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');   // headings
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');   // body text
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');   // placeholder

	// Suppress unused
	void fontMono;

	// Effective rail width
	let effectiveWidth = $derived((expanded || pinned) ? width : RAIL_WIDTH_COLLAPSED);
	let showLabels     = $derived(expanded || pinned);

	// Accordion expand state per item
	let accordionOpen = $state<Record<string, boolean>>({});

	// Nav item definitions
	const mainNav: NavItem[] = [
		{ id: 'dashboard', label: 'Dashboard', icon: LayoutDashboardIcon },
		{
			id: 'agents', label: 'Agents', icon: BotIcon,
			children: [
				{ id: 'agents-my',     label: 'My Agents' },
				{ id: 'agents-browse', label: 'Browse Library' },
				{ id: 'agents-create', label: 'Create Agent' }
			]
		},
		{
			id: 'skills', label: 'Skills', icon: ZapIcon,
			children: [
				{ id: 'skills-all',    label: 'All Skills' },
				{ id: 'skills-active', label: 'Active' },
				{ id: 'skills-create', label: 'New Skill' }
			]
		},
		{
			id: 'applications', label: 'Applications', icon: LayoutGridIcon,
			children: [
				{ id: 'apps-installed',  label: 'Installed' },
				{ id: 'apps-browse',     label: 'Browse' },
				{ id: 'apps-configure',  label: 'Configure' }
			]
		},
		{
			id: 'coding', label: 'Coding Assistant', icon: CodeIcon,
			children: [
				{ id: 'coding-review', label: 'Code Review' },
				{ id: 'coding-gen',    label: 'Code Generation' },
				{ id: 'coding-debug',  label: 'Debugger' }
			]
		},
		{
			id: 'personal', label: 'Personal Assistant', icon: UserIcon,
			children: [
				{ id: 'personal-tasks',    label: 'Tasks' },
				{ id: 'personal-calendar', label: 'Calendar' },
				{ id: 'personal-email',    label: 'Email' }
			]
		},
		{
			id: 'knowledge', label: 'Knowledge Base', icon: BookOpenIcon,
			children: [
				{ id: 'kb-docs',   label: 'Documents' },
				{ id: 'kb-search', label: 'Search' },
				{ id: 'kb-import', label: 'Import' }
			]
		},
		{
			id: 'tools', label: 'Tools', icon: WorkflowIcon,
			children: [
				{ id: 'flow', label: 'Flow' }
			]
		}
	];

	const bottomNav: NavItem[] = [
		{ id: 'settings', label: 'Settings', icon: SettingsIcon },
		{ id: 'about',    label: 'About',    icon: InfoIcon }
	];

	const user = { name: 'Alex Johnson', email: 'alex@example.com' };

	function isItemActive(item: NavItem): boolean {
		return !!activeMenu && activeMenu.itemId === item.id;
	}

	function isChildActive(child: NavChild): boolean {
		return !!activeMenu && activeMenu.childId === child.id;
	}

	function selectItem(item: NavItem, child?: NavChild) {
		onSelect({
			itemId:     item.id,
			childId:    child?.id,
			itemTitle:  item.label,
			childTitle: child?.label
		});
		if (child) return;
		if (item.children) {
			accordionOpen[item.id] = !accordionOpen[item.id];
		}
	}

	function toggleAccordion(id: string) {
		accordionOpen[id] = !accordionOpen[id];
	}

	// Hover background
	let hoverBg = $derived(darkMode ? 'rgba(45,51,72,0.6)' : 'rgba(228,230,235,0.7)');
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<aside
	class="flex-shrink-0 flex flex-col overflow-hidden relative"
	style="width:{effectiveWidth}px; background:{surface2}; border-right:1px solid {borderColor}; transition:width {RAIL_TRANSITION};"
	onmouseenter={() => onHoverChange(true)}
	onmouseleave={() => onHoverChange(false)}
>
	<!-- Pin / expand button at top -->
	<div
		class="flex items-center flex-shrink-0 px-2"
		style="height:48px; border-bottom:1px solid {borderColor};"
	>
		{#if showLabels}
			<div class="flex items-center justify-between w-full px-1">
				<span style="font-size:13px; font-weight:600; color:{accent};">Navigation</span>
				<button
					onclick={onTogglePin}
					class="flex items-center justify-center w-7 h-7 rounded-lg cursor-pointer transition-colors duration-150"
					style="color:{textMuted};"
					onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = accent; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; }}
					aria-label={pinned ? 'Unpin rail' : 'Pin rail'}
					title={pinned ? 'Unpin rail' : 'Pin rail'}
				>
					{#if pinned}
						<PanelLeftCloseIcon class="w-4 h-4" />
					{:else}
						<PanelLeftIcon class="w-4 h-4" />
					{/if}
				</button>
			</div>
		{:else}
			<button
				onclick={onTogglePin}
				class="flex items-center justify-center w-full h-8 rounded-lg cursor-pointer transition-colors duration-150"
				style="color:{textMuted};"
				onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = accent; }}
				onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; }}
				aria-label="Expand rail"
				title="Expand navigation"
			>
				<PanelLeftIcon class="w-5 h-5" />
			</button>
		{/if}
	</div>

	<!-- Main nav items (scrollable) -->
	<nav class="flex-1 overflow-y-auto py-2" style="scrollbar-width:thin; scrollbar-color:{borderColor} transparent;">
		{#each mainNav as item (item.id)}
			<div class="px-2 mb-0.5">
				<!-- Parent item button -->
				<button
					onclick={() => item.children ? toggleAccordion(item.id) : selectItem(item)}
					class="flex w-full items-center gap-3 rounded-lg cursor-pointer transition-colors duration-150"
					style="
						padding: {showLabels ? '8px 10px' : '9px 0'};
						justify-content: {showLabels ? 'flex-start' : 'center'};
						background: {isItemActive(item) ? accentTint : 'transparent'};
						color: {isItemActive(item) ? accent : textSecondary};
						border-left: {isItemActive(item) && showLabels ? '2px solid ' + accent : '2px solid transparent'};
					"
					onmouseenter={(e) => {
						const el = e.currentTarget as HTMLElement;
						if (!isItemActive(item)) el.style.background = hoverBg;
						el.style.color = textPrimary;
					}}
					onmouseleave={(e) => {
						const el = e.currentTarget as HTMLElement;
						if (!isItemActive(item)) el.style.background = 'transparent';
						el.style.color = isItemActive(item) ? accent : textSecondary;
					}}
					title={!showLabels ? item.label : undefined}
					aria-label={item.label}
				>
					<item.icon class="flex-shrink-0" style="width:20px; height:20px;" />
					{#if showLabels}
						<span class="flex-1 text-left truncate" style="font-size:14px; font-weight:500;">{item.label}</span>
						{#if item.children}
							<ChevronDownIcon
								class="flex-shrink-0 transition-transform duration-200"
								style="width:14px; height:14px; transform: rotate({accordionOpen[item.id] ? '180deg' : '0deg'});"
							/>
						{/if}
					{/if}
				</button>

				<!-- Sub-items (accordion, expanded rail only) -->
				{#if showLabels && item.children && accordionOpen[item.id]}
					<div class="mt-0.5 mb-1 ml-3" style="border-left:2px solid {borderColor};">
						{#each item.children as child (child.id)}
							<button
								onclick={() => selectItem(item, child)}
								class="flex w-full items-center gap-2 px-3 py-1.5 cursor-pointer transition-colors duration-150"
								style="
									color: {isChildActive(child) ? accent : textMuted};
									background: {isChildActive(child) ? accentTint : 'transparent'};
									font-size: 13px;
								"
								onmouseenter={(e) => {
									const el = e.currentTarget as HTMLElement;
									if (!isChildActive(child)) { el.style.background = hoverBg; el.style.color = textPrimary; }
								}}
								onmouseleave={(e) => {
									const el = e.currentTarget as HTMLElement;
									if (!isChildActive(child)) { el.style.background = 'transparent'; el.style.color = textMuted; }
								}}
							>
								<div class="w-1 h-1 rounded-full flex-shrink-0" style="background:currentColor; opacity:0.5;"></div>
								{child.label}
							</button>
						{/each}
					</div>
				{/if}
			</div>
		{/each}

		<!-- Separator -->
		<div class="mx-3 my-2" style="height:1px; background:{borderColor};"></div>

		<!-- Bottom nav -->
		{#each bottomNav as item (item.id)}
			<div class="px-2 mb-0.5">
				<button
					onclick={() => selectItem(item)}
					class="flex w-full items-center gap-3 rounded-lg cursor-pointer transition-colors duration-150"
					style="
						padding: {showLabels ? '8px 10px' : '9px 0'};
						justify-content: {showLabels ? 'flex-start' : 'center'};
						background: {isItemActive(item) ? accentTint : 'transparent'};
						color: {isItemActive(item) ? accent : textMuted};
						border-left: {isItemActive(item) && showLabels ? '2px solid ' + accent : '2px solid transparent'};
					"
					onmouseenter={(e) => {
						const el = e.currentTarget as HTMLElement;
						if (!isItemActive(item)) el.style.background = hoverBg;
						el.style.color = textPrimary;
					}}
					onmouseleave={(e) => {
						const el = e.currentTarget as HTMLElement;
						if (!isItemActive(item)) el.style.background = 'transparent';
						el.style.color = isItemActive(item) ? accent : textMuted;
					}}
					title={!showLabels ? item.label : undefined}
					aria-label={item.label}
				>
					<item.icon class="flex-shrink-0" style="width:20px; height:20px;" />
					{#if showLabels}
						<span class="flex-1 text-left truncate" style="font-size:14px; font-weight:500;">{item.label}</span>
					{/if}
				</button>
			</div>
		{/each}
	</nav>

	<!-- User section at bottom -->
	<div class="flex-shrink-0 p-2" style="border-top:1px solid {borderColor};">
		{#if showLabels}
			<div
				class="flex items-center gap-2 rounded-lg p-2"
				style="background:{hoverBg};"
			>
				<!-- Avatar -->
				<div
					class="flex items-center justify-center rounded-lg flex-shrink-0 text-xs font-semibold"
					style="width:32px; height:32px; background:{accentTint}; color:{accent}; border:1px solid {accent}30;"
				>
					{user.name.split(' ').map(n => n[0]).join('')}
				</div>
				<!-- Name + email -->
				<div class="flex-1 min-w-0">
					<div class="truncate" style="font-size:13px; font-weight:500; color:{textPrimary};">{user.name}</div>
					<div class="truncate" style="font-size:11px; color:{textMuted};">{user.email}</div>
				</div>
				<!-- Three-dots dropdown -->
				<DropdownMenu.Root>
					<DropdownMenu.Trigger>
						{#snippet child({ props })}
							<button
								{...props}
								class="flex items-center justify-center w-6 h-6 rounded-md cursor-pointer flex-shrink-0 transition-colors duration-150"
								style="color:{textMuted};"
								onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = textPrimary; (e.currentTarget as HTMLElement).style.background = borderColor; }}
								onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; (e.currentTarget as HTMLElement).style.background = 'transparent'; }}
								aria-label="User menu"
							>
								<MoreHorizontalIcon class="w-4 h-4" />
							</button>
						{/snippet}
					</DropdownMenu.Trigger>
					<DropdownMenu.Content class="min-w-44 rounded-lg" side="top" align="end" sideOffset={8}>
						<DropdownMenu.Item onclick={() => onSelect({ itemId: '__user_info__', itemTitle: 'User Info' })}>
							<UserCircle2Icon class="w-4 h-4 mr-2" />
							User Info
						</DropdownMenu.Item>
						<DropdownMenu.Item onclick={() => onSelect({ itemId: '__account__', itemTitle: 'Account' })}>
							<CreditCardIcon class="w-4 h-4 mr-2" />
							Account
						</DropdownMenu.Item>
						<DropdownMenu.Separator />
						<DropdownMenu.Item onclick={() => onSelect({ itemId: '__logout__', itemTitle: 'Logout' })}>
							<LogOutIcon class="w-4 h-4 mr-2" />
							Log Out
						</DropdownMenu.Item>
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			</div>
		{:else}
			<!-- Collapsed: avatar only with dropdown -->
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<button
							{...props}
							class="flex items-center justify-center w-full h-9 rounded-lg cursor-pointer"
							style="background:{accentTint}; color:{accent}; font-size:11px; font-weight:600;"
							aria-label="User menu"
							title={user.name}
						>
							{user.name.split(' ').map(n => n[0]).join('')}
						</button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content class="min-w-44 rounded-lg" side="right" align="end" sideOffset={8}>
					<DropdownMenu.Item onclick={() => onSelect({ itemId: '__user_info__', itemTitle: 'User Info' })}>
						<UserCircle2Icon class="w-4 h-4 mr-2" />
						User Info
					</DropdownMenu.Item>
					<DropdownMenu.Item onclick={() => onSelect({ itemId: '__account__', itemTitle: 'Account' })}>
						<CreditCardIcon class="w-4 h-4 mr-2" />
						Account
					</DropdownMenu.Item>
					<DropdownMenu.Separator />
					<DropdownMenu.Item onclick={() => onSelect({ itemId: '__logout__', itemTitle: 'Logout' })}>
						<LogOutIcon class="w-4 h-4 mr-2" />
						Log Out
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		{/if}
	</div>

	<!-- Resize handle (right edge, only visible when expanded/pinned) -->
	{#if showLabels}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="absolute right-0 top-0 bottom-0 cursor-col-resize"
			style="width:4px; z-index:10;"
			onmousedown={onWidthDragStart}
		></div>
	{/if}
</aside>

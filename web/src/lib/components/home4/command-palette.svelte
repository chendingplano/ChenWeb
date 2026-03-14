<script lang="ts">
	import * as Command from '$lib/components/ui/command/index.js';
	import LayoutDashboardIcon from '@lucide/svelte/icons/layout-dashboard';
	import BotIcon from '@lucide/svelte/icons/bot';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import LayoutGridIcon from '@lucide/svelte/icons/layout-grid';
	import CodeIcon from '@lucide/svelte/icons/code-2';
	import UserIcon from '@lucide/svelte/icons/user';
	import BookOpenIcon from '@lucide/svelte/icons/book-open';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import PlayIcon from '@lucide/svelte/icons/play';
	import UploadIcon from '@lucide/svelte/icons/upload';

	type ActiveSelection = {
		itemId: string;
		childId?: string;
		itemTitle: string;
		childTitle?: string;
	};

	let {
		darkMode,
		open,
		onClose,
		onSelect
	}: {
		darkMode: boolean;
		open: boolean;
		onClose: () => void;
		onSelect: (sel: ActiveSelection) => void;
	} = $props();

	// --- Colour tokens (all $derived from darkMode) ---
	let cardBg      = $derived(darkMode ? '#1E2535' : '#ffffff');       // card / panel background
	let border      = $derived(darkMode ? '#2D3348' : '#E4E6EB');       // border / divider lines
	let textMain    = $derived(darkMode ? '#E2E8F0' : '#111827');       // primary text
	let textMuted   = $derived(darkMode ? '#64748b' : '#94a3b8');       // muted text
	let accent      = $derived(darkMode ? '#818CF8' : '#6366F1');       // primary accent (indigo)

	let searchValue = $state('');

	// Suppress unused vars
	void textMain; void accent;

	const navItems: Array<{ sel: ActiveSelection; icon: any; label: string }> = [
		{ label: 'Dashboard', sel: { itemId: 'dashboard', itemTitle: 'Dashboard' }, icon: LayoutDashboardIcon },
		{ label: 'Agents', sel: { itemId: 'agents', itemTitle: 'Agents' }, icon: BotIcon },
		{ label: 'Skills', sel: { itemId: 'skills', itemTitle: 'Skills' }, icon: ZapIcon },
		{ label: 'Applications', sel: { itemId: 'applications', itemTitle: 'Applications' }, icon: LayoutGridIcon },
		{ label: 'Coding Assistant', sel: { itemId: 'coding', itemTitle: 'Coding Assistant' }, icon: CodeIcon },
		{ label: 'Personal Assistant', sel: { itemId: 'personal', itemTitle: 'Personal Assistant' }, icon: UserIcon },
		{ label: 'Knowledge Base', sel: { itemId: 'knowledge', itemTitle: 'Knowledge Base' }, icon: BookOpenIcon }
	];

	const quickActions: Array<{ label: string; icon: any; sel: ActiveSelection }> = [
		{ label: 'New Agent', icon: PlusIcon, sel: { itemId: 'agents', childId: 'agents-create', itemTitle: 'Agents', childTitle: 'Create Agent' } },
		{ label: 'Run Skill', icon: PlayIcon, sel: { itemId: 'skills', childId: 'skills-active', itemTitle: 'Skills', childTitle: 'Active' } },
		{ label: 'Import Doc', icon: UploadIcon, sel: { itemId: 'knowledge', childId: 'kb-import', itemTitle: 'Knowledge Base', childTitle: 'Import' } },
		{ label: 'Code Review', icon: CodeIcon, sel: { itemId: 'coding', childId: 'coding-review', itemTitle: 'Coding Assistant', childTitle: 'Code Review' } }
	];

	function handleSelect(sel: ActiveSelection) {
		onSelect(sel);
		onClose();
		searchValue = '';
	}

	function handleOverlayClick(e: MouseEvent) {
		if (e.target === e.currentTarget) onClose();
	}

	function handleOverlayKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			onClose();
			searchValue = '';
		}
	}

	function handleCardKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			onClose();
			searchValue = '';
		}
	}
</script>

{#if open}
	<!-- Full-screen overlay -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-start justify-center"
		style="background:rgba(0,0,0,0.50); padding-top:20vh;"
		onclick={handleOverlayClick}
		onkeydown={handleOverlayKeydown}
		role="presentation"
	>
		<!-- Centred card -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="w-full overflow-hidden rounded-xl shadow-2xl"
			style="max-width:560px; background:{cardBg}; border:1px solid {border};"
			onclick={(e) => e.stopPropagation()}
			onkeydown={handleCardKeydown}
			role="dialog"
			aria-modal="true"
			aria-label="Command palette"
		>
			<Command.Root>
				<!-- Search input — Command.Input includes its own search icon and border-b -->
				<div class="relative">
					<Command.Input
						placeholder="Search sections, actions..."
						class="pr-14"
					/>
					<kbd
						class="px-2 py-0.5 text-xs rounded-md absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none"
						style="background:rgba(129,140,248,0.10); color:{textMuted}; border:1px solid {border}; font-family:'Fira Code',monospace;"
					>Esc</kbd>
				</div>

				<Command.List style="max-height:360px; overflow-y:auto;">
					<Command.Empty>
						<div class="py-6 text-center text-sm" style="color:{textMuted};">No results found.</div>
					</Command.Empty>

					<!-- Navigation group -->
					<Command.Group heading="Navigation">
						{#each navItems as item}
							<Command.Item
								value={item.label}
								onSelect={() => handleSelect(item.sel)}
							>
								<item.icon class="w-4 h-4 flex-shrink-0" />
								<span>{item.label}</span>
							</Command.Item>
						{/each}
					</Command.Group>

					<Command.Separator />

					<!-- Quick Actions group -->
					<Command.Group heading="Quick Actions">
						{#each quickActions as action}
							<Command.Item
								value={action.label}
								onSelect={() => handleSelect(action.sel)}
							>
								<action.icon class="w-4 h-4 flex-shrink-0" />
								<span>{action.label}</span>
							</Command.Item>
						{/each}
					</Command.Group>
				</Command.List>
			</Command.Root>
		</div>
	</div>
{/if}

<script lang="ts">
	import ChevronsUpDownIcon from '@lucide/svelte/icons/chevrons-up-down';
	import ListFilterIcon from '@lucide/svelte/icons/list-filter';
	import NetworkIcon from '@lucide/svelte/icons/network';
	import ImageDownIcon from '@lucide/svelte/icons/image-down';
	import Settings2Icon from '@lucide/svelte/icons/settings-2';
	import CircleIcon from '@lucide/svelte/icons/circle';
	import SquareIcon from '@lucide/svelte/icons/square';

	type NodeStyle = 'circle' | 'rect';

	type Props = {
		darkMode?: boolean;
		nodeStyle?: NodeStyle;
		onExpandCollapseAll?: () => void;
		onFilter?: () => void;
		onExpandToLevel?: (level: number) => void;
		onExportPng?: () => void;
		onSettings?: () => void;
		onToggleNodeStyle?: () => void;
	};

	let {
		darkMode = true,
		nodeStyle,
		onExpandCollapseAll,
		onFilter,
		onExpandToLevel,
		onExportPng,
		onSettings,
		onToggleNodeStyle
	}: Props = $props();

	let expandLevelOpen = $state(false);
	let expandLevel = $state(2);

	let surface = $derived(darkMode ? 'rgba(15,23,42,0.55)' : 'rgba(255,255,255,0.72)');
	let borderColor = $derived(darkMode ? 'rgba(148,163,184,0.14)' : 'rgba(100,116,139,0.18)');
	let textColor = $derived(darkMode ? '#94a3b8' : '#64748b');
	let hoverBg = $derived(darkMode ? 'rgba(99,102,241,0.14)' : 'rgba(99,102,241,0.10)');
	let popoverBg = $derived(darkMode ? '#1e2a3a' : '#ffffff');

	function handleExpandToLevel(event: MouseEvent) {
		event.stopPropagation();
		expandLevelOpen = !expandLevelOpen;
	}

	function commitExpandToLevel() {
		expandLevelOpen = false;
		onExpandToLevel?.(expandLevel);
	}

	function closePopoverOnOutside(event: MouseEvent) {
		const target = event.target as HTMLElement;
		if (!target.closest('.toolbar-expand-level-wrap')) {
			expandLevelOpen = false;
		}
	}
</script>

<svelte:window onclick={closePopoverOnOutside} />

<div
	class="view-toolbar"
	style={`--surface:${surface}; --border-c:${borderColor}; --text-c:${textColor}; --hover:${hoverBg}; --pop:${popoverBg};`}
>
	<div class="toolbar-group">
		<button
			type="button"
			class="toolbar-btn"
			title="Expand / Collapse All Levels"
			onclick={() => onExpandCollapseAll?.()}
		>
			<ChevronsUpDownIcon class="tb-icon" />
		</button>

		<div class="toolbar-sep" aria-hidden="true"></div>

		<button
			type="button"
			class="toolbar-btn"
			title="Filter Nodes in Current Level"
			onclick={() => onFilter?.()}
		>
			<ListFilterIcon class="tb-icon" />
		</button>

		<div class="toolbar-sep" aria-hidden="true"></div>

		<div class="toolbar-expand-level-wrap">
			<button
				type="button"
				class="toolbar-btn"
				class:active={expandLevelOpen}
				title="Expand Selected Node to Level"
				onclick={handleExpandToLevel}
			>
				<NetworkIcon class="tb-icon" />
			</button>

			{#if expandLevelOpen}
				<div class="level-popover" role="dialog" aria-label="Expand to level">
					<div class="level-label">Expand to depth</div>
					<div class="level-row">
						<input
							type="range"
							min="1"
							max="6"
							step="1"
							bind:value={expandLevel}
							class="level-slider"
						/>
						<span class="level-value">{expandLevel}</span>
					</div>
					<button type="button" class="level-apply" onclick={commitExpandToLevel}>Apply</button>
				</div>
			{/if}
		</div>

		<div class="toolbar-sep" aria-hidden="true"></div>

		<button
			type="button"
			class="toolbar-btn"
			title="Export to PNG"
			onclick={() => onExportPng?.()}
		>
			<ImageDownIcon class="tb-icon" />
		</button>

		<div class="toolbar-sep" aria-hidden="true"></div>

		<button
			type="button"
			class="toolbar-btn"
			title="Settings"
			onclick={() => onSettings?.()}
		>
			<Settings2Icon class="tb-icon" />
		</button>

		{#if onToggleNodeStyle !== undefined}
			<div class="toolbar-sep" aria-hidden="true"></div>

			<button
				type="button"
				class="toolbar-btn"
				class:active={nodeStyle === 'circle'}
				title={nodeStyle === 'circle' ? 'Switch to rectangle nodes' : 'Switch to circle nodes'}
				onclick={() => onToggleNodeStyle?.()}
			>
				{#if nodeStyle === 'circle'}
					<CircleIcon class="tb-icon" />
				{:else}
					<SquareIcon class="tb-icon" />
				{/if}
			</button>
		{/if}
	</div>
</div>

<style>
	.view-toolbar {
		display: flex;
		align-items: center;
		margin-bottom: 0.75rem;
	}

	.toolbar-group {
		display: inline-flex;
		align-items: center;
		gap: 0;
		border-radius: 12px;
		border: 1px solid var(--border-c);
		background: var(--surface);
		backdrop-filter: blur(8px);
		padding: 3px;
	}

	.toolbar-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 32px;
		height: 32px;
		border-radius: 8px;
		border: none;
		background: transparent;
		color: var(--text-c);
		cursor: pointer;
		transition:
			background 120ms ease,
			color 120ms ease;
	}

	.toolbar-btn:hover,
	.toolbar-btn.active {
		background: var(--hover);
		color: #818cf8;
	}

	:global(.tb-icon) {
		width: 15px;
		height: 15px;
		flex-shrink: 0;
		pointer-events: none;
	}

	.toolbar-sep {
		width: 1px;
		height: 18px;
		background: var(--border-c);
		margin: 0 2px;
		flex-shrink: 0;
	}

	.toolbar-expand-level-wrap {
		position: relative;
	}

	.level-popover {
		position: absolute;
		top: calc(100% + 8px);
		left: 50%;
		transform: translateX(-50%);
		z-index: 40;
		min-width: 180px;
		border-radius: 12px;
		border: 1px solid var(--border-c);
		background: var(--pop);
		padding: 0.75rem;
		box-shadow: 0 12px 40px rgba(0, 0, 0, 0.28);
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.level-label {
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--text-c);
	}

	.level-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.level-slider {
		flex: 1;
		accent-color: #818cf8;
		cursor: pointer;
	}

	.level-value {
		min-width: 1.25rem;
		text-align: center;
		font-size: 0.9rem;
		font-weight: 700;
		color: #818cf8;
	}

	.level-apply {
		align-self: flex-end;
		padding: 0.35rem 0.8rem;
		border-radius: 8px;
		border: none;
		background: linear-gradient(135deg, #4f46e5, #6366f1);
		color: #fff;
		font-size: 0.8rem;
		font-weight: 700;
		cursor: pointer;
		transition: opacity 100ms;
	}

	.level-apply:hover {
		opacity: 0.88;
	}
</style>

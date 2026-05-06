<script lang="ts">
	import ChevronsUpDownIcon from '@lucide/svelte/icons/chevrons-up-down';
	import ListFilterIcon from '@lucide/svelte/icons/list-filter';
	import FilterXIcon from '@lucide/svelte/icons/filter-x';
	import NetworkIcon from '@lucide/svelte/icons/network';
	import ImageDownIcon from '@lucide/svelte/icons/image-down';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import CircleIcon from '@lucide/svelte/icons/circle';
	import SquareIcon from '@lucide/svelte/icons/square';
	import Minimize2Icon from '@lucide/svelte/icons/minimize-2';

	type NodeStyle = 'circle' | 'rect';

	export type GraphSettings = {
		defaultExpandDepth: number;
		showInfoBlock: boolean;
	};

	type Props = {
		darkMode?: boolean;
		nodeStyle?: NodeStyle;
		onExpandCollapseAll?: () => void;
		onFilter?: () => void;
		filterDisabled?: boolean;
		onResetFilter?: () => void;
		resetFilterDisabled?: boolean;
		onExpandToLevel?: (level: number) => void;
		onCollapseSelected?: () => void;
		collapseSelectedDisabled?: boolean;
		onExportPng?: () => void;
		settings?: GraphSettings;
		onSettingsChange?: (patch: Partial<GraphSettings>) => void;
		onToggleNodeStyle?: () => void;
	};

	let {
		darkMode = true,
		nodeStyle,
		onExpandCollapseAll,
		onFilter,
		filterDisabled = false,
		onResetFilter,
		resetFilterDisabled = true,
		onExpandToLevel,
		onCollapseSelected,
		collapseSelectedDisabled = true,
		onExportPng,
		settings,
		onSettingsChange,
		onToggleNodeStyle
	}: Props = $props();

	let expandLevelOpen = $state(false);
	let expandLevel = $state(2);
	let settingsOpen = $state(false);

	$effect(() => {
		const max = settings?.defaultExpandDepth ?? 6;
		if (expandLevel > max) expandLevel = max;
	});

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

	function handleSettings(event: MouseEvent) {
		event.stopPropagation();
		settingsOpen = !settingsOpen;
	}

	function closePopoverOnOutside(event: MouseEvent) {
		const target = event.target as HTMLElement;
		if (!target.closest('.toolbar-expand-level-wrap')) {
			expandLevelOpen = false;
		}
		if (!target.closest('.toolbar-settings-wrap')) {
			settingsOpen = false;
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
			disabled={filterDisabled}
			aria-disabled={filterDisabled}
			title="Filter Nodes in Current Level"
			onclick={() => {
				if (!filterDisabled) onFilter?.();
			}}
		>
			<ListFilterIcon class="tb-icon" />
		</button>

		<button
			type="button"
			class="toolbar-btn"
			disabled={resetFilterDisabled}
			aria-disabled={resetFilterDisabled}
			title="Reset Current Level Filter"
			onclick={() => {
				if (!resetFilterDisabled) onResetFilter?.();
			}}
		>
			<FilterXIcon class="tb-icon" />
		</button>

		<div class="toolbar-sep" aria-hidden="true"></div>

		<div class="toolbar-expand-level-wrap">
			<button
				type="button"
				class="toolbar-btn"
				class:active={expandLevelOpen}
				title="Expand Selected Node"
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
							max={settings?.defaultExpandDepth ?? 6}
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

		<button
			type="button"
			class="toolbar-btn"
			disabled={collapseSelectedDisabled}
			aria-disabled={collapseSelectedDisabled}
			title="Collapse Selected Node"
			onclick={() => {
				if (!collapseSelectedDisabled) onCollapseSelected?.();
			}}
		>
			<Minimize2Icon class="tb-icon" />
		</button>

		<div class="toolbar-sep" aria-hidden="true"></div>

		<button
			type="button"
			class="toolbar-btn"
			title="Export to PNG"
			onclick={() => onExportPng?.()}
		>
			<ImageDownIcon class="tb-icon" />
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

		<div class="toolbar-sep" aria-hidden="true"></div>

		<div class="toolbar-settings-wrap">
			<button
				type="button"
				class="toolbar-btn"
				class:active={settingsOpen}
				title="Settings"
				onclick={handleSettings}
			>
				<SettingsIcon class="tb-icon" />
			</button>

			{#if settingsOpen}
				<div class="settings-popover" role="dialog" aria-label="Settings">
					<div class="level-label">Settings</div>

					<div class="settings-field">
						<label class="settings-field-label" for="setting-expand-depth"
							>Default Expand Selected Node Depth</label
						>
						<input
							id="setting-expand-depth"
							type="number"
							min="1"
							max="20"
							value={settings?.defaultExpandDepth ?? 6}
							oninput={(e) => {
								const v = parseInt((e.target as HTMLInputElement).value);
								if (!isNaN(v) && v >= 1) onSettingsChange?.({ defaultExpandDepth: v });
							}}
							class="settings-number-input"
						/>
						<div class="settings-help">The maximum depth of expanding the selected node.</div>
					</div>

					<div class="settings-field">
						<div class="settings-checkbox-row">
							<input
								id="setting-show-info"
								type="checkbox"
								checked={settings?.showInfoBlock ?? false}
								onchange={(e) => {
									onSettingsChange?.({ showInfoBlock: (e.target as HTMLInputElement).checked });
								}}
								class="settings-checkbox"
							/>
							<label class="settings-field-label" for="setting-show-info"
								>Show Information Block</label
							>
						</div>
						<div class="settings-help">
							Control whether to show the information when the mouse hovers over a node.
						</div>
					</div>
				</div>
			{/if}
		</div>
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
		position: relative;
		z-index: 10;
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

	.toolbar-btn:disabled {
		opacity: 0.38;
		cursor: not-allowed;
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

	.toolbar-settings-wrap {
		position: relative;
	}

	.settings-popover {
		position: absolute;
		top: calc(100% + 8px);
		right: 0;
		z-index: 40;
		min-width: 220px;
		border-radius: 12px;
		border: 1px solid var(--border-c);
		background: var(--pop);
		padding: 0.75rem;
		box-shadow: 0 12px 40px rgba(0, 0, 0, 0.28);
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.settings-field {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.settings-field-label {
		font-size: 0.78rem;
		font-weight: 600;
		color: var(--text-c);
	}

	.settings-help {
		font-size: 0.68rem;
		color: var(--text-c);
		opacity: 0.7;
		line-height: 1.4;
	}

	.settings-number-input {
		width: 72px;
		padding: 0.25rem 0.4rem;
		border-radius: 6px;
		border: 1px solid var(--border-c);
		background: transparent;
		color: #818cf8;
		font-size: 0.9rem;
		font-weight: 700;
		text-align: center;
	}

	.settings-checkbox-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.settings-checkbox {
		accent-color: #818cf8;
		width: 15px;
		height: 15px;
		cursor: pointer;
		flex-shrink: 0;
	}
</style>

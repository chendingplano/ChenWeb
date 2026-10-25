<script lang="ts">
	// Shows one kb.metrics row: its name, then the Metric / Metadata / Context /
	// Grounding / Reasoning groups, each as a name-value list. See metric-detail.ts and
	// KnowledgeStore/doc-repo/modules/module-metric-detail-visualizer.md for usage.
	import { m } from '$lib/paraglide/messages.js';
	import type { KbMetricRecord, RawLine } from '$lib/services/kbService';
	import NameValueVisualizer from '../name-value/name-value-visualizer.svelte';
	import { buildMetricDetailGroups, metricTitle, type MetricDetailGroupKey } from './metric-detail';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import BookOpenIcon from '@lucide/svelte/icons/book-open';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import MapPinIcon from '@lucide/svelte/icons/map-pin';
	import TagIcon from '@lucide/svelte/icons/tag';
	import TrendingUpIcon from '@lucide/svelte/icons/trending-up';

	let {
		metric,
		rawLines = [],
		position,
		onClose,
		onPrev,
		onNext
	}: {
		metric: KbMetricRecord;
		// Lines of the metric's document; Grounding shows the ones the metric cites.
		rawLines?: RawLine[];
		// Place of the metric in the caller's list (index -1 = not in it); shows "3 / 27".
		position?: { index: number; total: number };
		// The toolbar appears when onClose or position is given. A missing
		// onPrev/onNext disables that button.
		onClose?: () => void;
		onPrev?: () => void;
		onNext?: () => void;
	} = $props();

	const GROUP_ICONS: Record<MetricDetailGroupKey, typeof ActivityIcon> = {
		metric: TrendingUpIcon,
		metadata: BookOpenIcon,
		context: TagIcon,
		grounding: MapPinIcon,
		reasoning: ActivityIcon
	};

	const title = $derived(metricTitle(metric));
	const groups = $derived(buildMetricDetailGroups(metric, rawLines));
</script>

<div class="metric-detail select-text">
	{#if onClose || position}
		<div class="md-toolbar">
			{#if onClose}
				<button type="button" class="md-button md-close" onclick={onClose} title={m.metric_detail_close_title()}>
					<ArrowLeftIcon size={13} />
					<span>{m.metric_detail_close()}</span>
				</button>
			{/if}
			{#if position}
				<div class="md-nav">
					<button
						type="button"
						class="md-button md-nav-button"
						disabled={!onPrev}
						onclick={onPrev}
						title={m.metric_detail_previous()}
						aria-label={m.metric_detail_previous()}><ChevronLeftIcon size={13} /></button
					>
					<span class="md-nav-position">
						{position.index >= 0 ? position.index + 1 : '—'} / {position.total}
					</span>
					<button
						type="button"
						class="md-button md-nav-button"
						disabled={!onNext}
						onclick={onNext}
						title={m.metric_detail_next()}
						aria-label={m.metric_detail_next()}><ChevronRightIcon size={13} /></button
					>
				</div>
			{/if}
		</div>
	{/if}
	<div class="md-body">
		<header class="md-header">
			<h2 class="md-name">{title.name}</h2>
			{#if title.subname}<div class="md-subname">{title.subname}</div>{/if}
		</header>
		{#each groups as group (group.key)}
			{@const Icon = GROUP_ICONS[group.key]}
			<section class="md-group" class:md-group-empty={group.filled === 0}>
				<div class="md-group-head">
					<span class="md-group-icon"><Icon size={16} /></span>
					<h3 class="md-group-label">{group.label}</h3>
					<span class="md-group-count">{group.filled}/{group.total}</span>
				</div>
				<NameValueVisualizer fields={group.fields} emptyText="—" labelWidth="10rem" ariaLabel={group.label} />
			</section>
		{/each}
	</div>
</div>

<style>
	/* Colours and faces come from the page's variables, with neutral fallbacks. */
	.metric-detail {
		--md-line: var(--ink-line, color-mix(in srgb, currentColor 18%, transparent));
		--md-line-soft: var(--ink-line-soft, var(--md-line));
		--md-accent: var(--brass, var(--accent, currentColor));
		--md-muted: var(--text-muted, color-mix(in srgb, currentColor 60%, transparent));
		--md-bar-bg: var(--panel-bg-alt, transparent);
		--md-button-bg: var(--panel-bg, transparent);
		--md-serif: var(--font-serif, Georgia, serif);
		--md-mono: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
		display: flex;
		flex-direction: column;
		flex: 1;
		min-width: 0;
		min-height: 0;
		color: var(--text-primary, inherit);
	}
	.md-toolbar {
		display: flex;
		flex-wrap: wrap;
		flex-shrink: 0;
		align-items: center;
		gap: 10px;
		padding: 8px 12px;
		border-bottom: 1px solid var(--md-line);
		background: var(--md-bar-bg);
	}
	.md-button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		border: 1px solid var(--md-line);
		border-radius: 6px;
		background: var(--md-button-bg);
		color: var(--text-secondary, inherit);
		cursor: pointer;
		transition:
			border-color 120ms,
			color 120ms,
			opacity 120ms;
	}
	.md-button:not(:disabled):hover {
		border-color: var(--md-accent);
		color: var(--md-accent);
	}
	.md-button:focus-visible {
		outline: 2px solid var(--md-accent);
		outline-offset: 2px;
	}
	.md-button:disabled {
		opacity: 0.35;
		cursor: not-allowed;
	}
	.md-close {
		gap: 5px;
		padding: 4px 10px 4px 6px;
		font-family: var(--md-mono);
		font-size: 11px;
		letter-spacing: 0.06em;
	}
	.md-nav {
		display: flex;
		align-items: center;
		gap: 4px;
	}
	.md-nav-button {
		width: 26px;
		height: 26px;
		padding: 0;
	}
	.md-nav-position {
		min-width: 56px;
		padding: 0 6px;
		font-family: var(--md-mono);
		font-size: 11px;
		color: var(--md-muted);
		text-align: center;
	}
	.md-body {
		flex: 1 1 auto;
		min-height: 0;
		overflow-y: auto;
		padding: 16px 16px 40px;
		scrollbar-width: thin;
		scrollbar-color: var(--md-line) transparent;
	}
	.md-header {
		padding-bottom: 16px;
		border-bottom: 1px solid var(--md-line);
		margin-bottom: 4px;
	}
	.md-name {
		margin: 0;
		font-family: var(--md-serif);
		font-size: 22px;
		font-weight: 500;
		line-height: 1.25;
	}
	.md-subname {
		margin-top: 4px;
		font-family: var(--md-mono);
		font-size: 11px;
		letter-spacing: 0.04em;
		color: var(--md-muted);
	}
	.md-group {
		margin-top: 10px;
		padding: 12px 14px;
		border: 1px solid var(--md-line-soft);
		border-radius: 10px;
		background: color-mix(in srgb, var(--md-bar-bg) 95%, white);
	}
	.md-group-empty {
		opacity: 0.55;
	}
	.md-group-head {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 10px;
	}
	.md-group-icon {
		display: inline-flex;
		flex-shrink: 0;
		color: var(--md-accent);
	}
	.md-group-label {
		flex: 1;
		margin: 0;
		font-family: var(--md-serif);
		font-size: 14px;
		font-weight: 600;
		letter-spacing: 0.01em;
		color: var(--md-accent);
	}
	.md-group-count {
		font-family: var(--md-mono);
		font-size: 10px;
		letter-spacing: 0.06em;
		color: var(--md-muted);
	}
</style>

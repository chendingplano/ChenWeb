<script lang="ts">
	import { browser } from '$app/environment';
	import { fly } from 'svelte/transition';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import UsersIcon from '@lucide/svelte/icons/users';
	import PlayIcon from '@lucide/svelte/icons/play';
	import RouteIcon from '@lucide/svelte/icons/route';
	import FlagIcon from '@lucide/svelte/icons/flag';
	import LinkIcon from '@lucide/svelte/icons/link';
	import KbInputRecordBrowser from './kb-input-record-browser.svelte';
	import PdfViewWindow from './pdf-view-window.svelte';
	import { knowledgeStoreState } from './knowledge-store-state.svelte';
	import {
		listKbSceneBlocks,
		type KbInputRecord,
		type KbSceneBlockRecord
	} from '$lib/services/kbService';

	let {
		darkMode = true,
		browserInstanceKey = 'scene-blocks',
		scopeToActiveStore = false,
		heroEyebrow = 'Subject Wiki',
		heroTitle = 'Scene Blocks',
		heroDescription = 'Inspect the event-driven scenes an LLM extracted from each document: who acts, what triggers the scene, how it unfolds, and how it resolves.'
	}: {
		darkMode?: boolean;
		browserInstanceKey?: string;
		scopeToActiveStore?: boolean;
		heroEyebrow?: string;
		heroTitle?: string;
		heroDescription?: string;
	} = $props();

	let panelBg = $derived(darkMode ? '#161c2b' : '#ffffff');
	let panelAlt = $derived(darkMode ? '#0f172a' : '#eef2ff');
	let border = $derived(darkMode ? '#2b3548' : '#dbe3f0');
	let textMain = $derived(darkMode ? '#e2e8f0' : '#0f172a');
	let textMuted = $derived(darkMode ? '#94a3b8' : '#64748b');
	let accent = $derived(darkMode ? '#22c55e' : '#16a34a');

	const LIST_MIN = 360;
	const LIST_MAX = 860;
	const LIST_DEFAULT = 560;
	const listWidthKey = $derived(`scene-blocks:list-width:${browserInstanceKey}`);

	let selectedRecordId = $state<number | null>(null);
	let recordCache = $state<Record<number, KbInputRecord>>({});
	let blocks = $state<KbSceneBlockRecord[]>([]);
	let loading = $state(false);
	let loadError = $state('');
	let expandedBlockId = $state<number | null>(null);
	let listWidth = $state(LIST_DEFAULT);
	let resizing = $state(false);
	let showDiscriminators = $state(false);

	let docPage = $state(1);
	let pdfZoom = $state(0.6);
	let pdfNumPages = $state(0);

	let activeRecord = $derived(
		selectedRecordId != null ? (recordCache[selectedRecordId] ?? null) : null
	);
	let viewerInputId = $derived(activeRecord?.id ?? null);
	let viewerFileUrl = $derived(viewerInputId ? `/api/v1/kb/inputs/${viewerInputId}/file` : '');
	let viewerIsPdf = $derived(
		(activeRecord?.file_name ?? '').trim().toLowerCase().endsWith('.pdf')
	);
	let activeBlock = $derived(
		expandedBlockId != null ? (blocks.find((b) => b.id === expandedBlockId) ?? null) : null
	);
	let avgConfidence = $derived(
		blocks.length
			? Math.round(
					(blocks.reduce((sum, b) => sum + (Number(b.confidence) || 0), 0) / blocks.length) * 100
				)
			: 0
	);

	function clampListWidth(value: number) {
		if (!Number.isFinite(value)) return LIST_DEFAULT;
		return Math.max(LIST_MIN, Math.min(LIST_MAX, Math.round(value)));
	}

	$effect(() => {
		if (!browser) return;
		const saved = Number(localStorage.getItem(listWidthKey));
		if (Number.isFinite(saved) && saved > 0) listWidth = clampListWidth(saved);
	});

	function persistListWidth(next: number) {
		listWidth = clampListWidth(next);
		if (browser) localStorage.setItem(listWidthKey, String(listWidth));
	}

	async function handleRecordSelect(record: KbInputRecord) {
		recordCache = { ...recordCache, [record.id]: record };
		if (selectedRecordId === record.id && blocks.length) return;
		selectedRecordId = record.id;
		expandedBlockId = null;
		showDiscriminators = false;
		loading = true;
		loadError = '';
		blocks = [];
		try {
			const response = await listKbSceneBlocks(record.id);
			blocks = response.results ?? [];
		} catch (error) {
			blocks = [];
			loadError =
				error instanceof Error ? error.message : 'Failed to load scene blocks for this record';
		} finally {
			loading = false;
		}
	}

	function toggleBlock(id: number) {
		expandedBlockId = expandedBlockId === id ? null : id;
		showDiscriminators = false;
		if (expandedBlockId === id) docPage = 1;
	}

	function confidenceBand(value: number) {
		if (value >= 0.8) return 'high';
		if (value >= 0.5) return 'mid';
		return 'low';
	}

	function startResize(event: PointerEvent) {
		event.preventDefault();
		const startX = event.clientX;
		const startWidth = listWidth;
		resizing = true;
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		const move = (e: PointerEvent) => persistListWidth(startWidth + (e.clientX - startX));
		const up = () => {
			resizing = false;
			document.body.style.cursor = '';
			document.body.style.userSelect = '';
			window.removeEventListener('pointermove', move);
			window.removeEventListener('pointerup', up);
			window.removeEventListener('pointercancel', up);
		};
		window.addEventListener('pointermove', move);
		window.addEventListener('pointerup', up, { once: true });
		window.addEventListener('pointercancel', up, { once: true });
	}

	function onResizerKeydown(event: KeyboardEvent) {
		if (event.key === 'ArrowLeft') {
			event.preventDefault();
			persistListWidth(listWidth - 24);
		} else if (event.key === 'ArrowRight') {
			event.preventDefault();
			persistListWidth(listWidth + 24);
		}
	}

	const STRING_GROUPS = [
		{ key: 'setup', label: 'Setup', icon: PlayIcon, fields: ['preconditions', 'triggers', 'states'] },
		{ key: 'flow', label: 'Flow', icon: RouteIcon, fields: ['decisions', 'constraints'] },
		{
			key: 'resolution',
			label: 'Resolution',
			icon: FlagIcon,
			fields: ['outcomes', 'failure_modes', 'root_causes', 'resolutions']
		}
	] as const;

	const FIELD_LABELS: Record<string, string> = {
		preconditions: 'Preconditions',
		triggers: 'Triggers',
		states: 'States',
		decisions: 'Decisions',
		constraints: 'Constraints',
		outcomes: 'Outcomes',
		failure_modes: 'Failure modes',
		root_causes: 'Root causes',
		resolutions: 'Resolutions'
	};

	function arr(value: unknown): any[] {
		return Array.isArray(value) ? value : [];
	}
	function strList(block: KbSceneBlockRecord, field: string): string[] {
		return arr((block as any)[field]).filter((v) => typeof v === 'string' && v.trim() !== '');
	}
	function groupHasContent(block: KbSceneBlockRecord, fields: readonly string[]) {
		return fields.some((f) => strList(block, f).length > 0);
	}
	function sortedActions(block: KbSceneBlockRecord) {
		return [...arr(block.actions)].sort(
			(a, b) => (Number(a?.sequence) || 0) - (Number(b?.sequence) || 0)
		);
	}
</script>

<div
	class="scene-shell"
	class:resizing
	style={`--panel:${panelBg}; --panel-alt:${panelAlt}; --border:${border}; --text:${textMain}; --muted:${textMuted}; --accent:${accent}; --panel-bg:${panelBg}; --panel-bg-alt:${panelAlt}; --ink-line:${border}; --ink-line-soft:rgba(148,163,184,0.16);`}
>
	<div class="hero">
		<div class="hero-copy">
			<div class="eyebrow">{heroEyebrow}</div>
			<h2>{heroTitle}</h2>
			<p>{heroDescription}</p>
		</div>
		{#if activeRecord && !loading && blocks.length}
			<div class="hero-stat" aria-label="Scene block summary for the selected record">
				<span class="hero-stat-count">{blocks.length}</span>
				<span class="hero-stat-label">scene {blocks.length === 1 ? 'block' : 'blocks'}</span>
				<span class="hero-stat-sep" aria-hidden="true">·</span>
				<span class="hero-stat-conf">avg confidence {avgConfidence}%</span>
			</div>
		{/if}
	</div>

	<div class="workspace">
		<KbInputRecordBrowser
			{darkMode}
			instanceKey={browserInstanceKey}
			title="kb.inputs"
			subtitle="Search, filter, and select a record to inspect its extracted scene blocks."
			emptyTitle="No records found."
			emptySubtitle="Use Search or Retrieve to browse kb.inputs for scene blocks."
			{scopeToActiveStore}
			{selectedRecordId}
			onSelect={handleRecordSelect}
		/>

		<div class="right-panel">
			<div class="right-tabs">
				<div class="tab active">Scene Blocks</div>
				{#if activeRecord}
					<div class="tab passive" title={activeRecord.file_name ?? ''}>
						{activeRecord.title?.trim() ||
							activeRecord.file_name?.trim() ||
							`Record #${activeRecord.id}`}
					</div>
				{/if}
			</div>

			{#if !activeRecord}
				<div class="empty-state">
					<div class="empty-title">Select a record</div>
					<div class="empty-copy">
						Choose a document from the left to read the scene blocks extracted from it.
					</div>
				</div>
			{:else}
				<div class="detail-grid">
					<div
						class="scene-card"
						style="width:{listWidth}px; flex:0 0 {listWidth}px;"
					>
						<div class="scene-card-head">
							<div class="eyebrow">Extracted scenes</div>
							<div class="scene-card-count">
								{#if loading}loading…{:else}{blocks.length} total{/if}
							</div>
						</div>

						<div class="scene-list">
							{#if loading}
								{#each Array(4) as _, i (i)}
									<div class="skeleton-row" style={`animation-delay:${i * 60}ms`}>
										<div class="sk sk-pill"></div>
										<div class="sk sk-title"></div>
										<div class="sk sk-line"></div>
									</div>
								{/each}
							{:else if loadError}
								<div class="error-card">
									<div class="error-title">Couldn't load scene blocks</div>
									<div class="error-copy">{loadError}</div>
								</div>
							{:else if blocks.length === 0}
								<div class="empty-state">
									<div class="empty-title">No scene blocks yet</div>
									<div class="empty-copy">
										This record has no entries in <code>kb.scene_objects</code>. Scene blocks
										are produced by the generate-scene-blocks processor once the document is
										processed.
									</div>
								</div>
							{:else}
								{#each blocks as block, idx (block.id)}
									{@const open = expandedBlockId === block.id}
									{@const conf = Number(block.confidence) || 0}
									<div class="scene-block" class:open>
										<button
											type="button"
											class="scene-row"
											aria-expanded={open}
											onclick={() => toggleBlock(block.id)}
										>
											<span class="row-index mono">{String(idx + 1).padStart(2, '0')}</span>
											<span class="chevron" class:open>
												<ChevronRightIcon class="h-3.5 w-3.5" />
											</span>
											<span class="row-main">
												<span class="row-top">
													{#if block.scene_type?.trim()}
														<span class="scene-type">{block.scene_type}</span>
													{/if}
													<span class="row-title">
														{block.title?.trim() || block.scene_id || `Scene ${idx + 1}`}
													</span>
												</span>
												{#if block.summary?.trim()}
													<span class="row-summary">{block.summary}</span>
												{/if}
												{#if strList(block, 'keywords').length}
													<span class="row-keywords">
														{#each strList(block, 'keywords').slice(0, 6) as kw}
															<span class="kw">{kw}</span>
														{/each}
													</span>
												{/if}
											</span>
											<span class="row-conf" title="Extraction confidence">
												<span class="conf-meter conf-{confidenceBand(conf)}">
													<span class="conf-fill" style="width:{Math.round(conf * 100)}%"></span>
												</span>
												<span class="conf-num mono">{Math.round(conf * 100)}%</span>
											</span>
										</button>

										{#if open}
											<div class="scene-detail" transition:fly={{ y: -6, duration: 160 }}>
												{#if arr(block.actors).length || arr(block.resources).length}
													<section class="grp">
														<header class="grp-head">
															<UsersIcon class="h-3.5 w-3.5" />
															<span>Cast</span>
														</header>
														{#if arr(block.actors).length}
															<div class="grp-sub">Actors</div>
															<div class="chips">
																{#each arr(block.actors) as actor}
																	<span class="entity">
																		{#if actor?.type}<span class="entity-type">{actor.type}</span>{/if}
																		<span class="entity-name">{actor?.name ?? '—'}</span>
																	</span>
																{/each}
															</div>
														{/if}
														{#if arr(block.resources).length}
															<div class="grp-sub">Resources</div>
															<div class="chips">
																{#each arr(block.resources) as res}
																	<span class="entity">
																		{#if res?.type}<span class="entity-type">{res.type}</span>{/if}
																		<span class="entity-name">{res?.name ?? '—'}</span>
																	</span>
																{/each}
															</div>
														{/if}
													</section>
												{/if}

												{#each STRING_GROUPS as g (g.key)}
													{#if groupHasContent(block, g.fields) || (g.key === 'flow' && sortedActions(block).length)}
														<section class="grp">
															<header class="grp-head">
																<g.icon class="h-3.5 w-3.5" />
																<span>{g.label}</span>
															</header>
															{#if g.key === 'flow' && sortedActions(block).length}
																<div class="grp-sub">Actions</div>
																<ol class="actions">
																	{#each sortedActions(block) as act, ai}
																		<li>
																			<span class="act-seq mono">
																				{String(Number(act?.sequence) || ai + 1).padStart(2, '0')}
																			</span>
																			<span class="act-body">
																				{#if act?.actor}<span class="act-actor">{act.actor}</span>{/if}
																				<span class="act-text">{act?.action ?? ''}</span>
																			</span>
																		</li>
																	{/each}
																</ol>
															{/if}
															{#each g.fields as field}
																{#if strList(block, field).length}
																	<div class="grp-sub">{FIELD_LABELS[field]}</div>
																	<ul class="lines">
																		{#each strList(block, field) as item}
																			<li>{item}</li>
																		{/each}
																	</ul>
																{/if}
															{/each}
														</section>
													{/if}
												{/each}

												{#if arr(block.relationships).length || arr(block.source_refs).length || arr(block.discriminators).length}
													<section class="grp">
														<header class="grp-head">
															<LinkIcon class="h-3.5 w-3.5" />
															<span>Links & evidence</span>
														</header>
														{#if arr(block.relationships).length}
															<div class="grp-sub">Relationships</div>
															<div class="rels">
																{#each arr(block.relationships) as rel}
																	<span class="rel">
																		<span class="rel-type">{rel?.type ?? 'related'}</span>
																		<span class="rel-arrow" aria-hidden="true">→</span>
																		<span class="rel-target">{rel?.target ?? '—'}</span>
																	</span>
																{/each}
															</div>
														{/if}
														{#if arr(block.source_refs).length}
															<div class="grp-sub">Source evidence</div>
															<ul class="srcrefs">
																{#each arr(block.source_refs) as ref}
																	<li>
																		<span class="src-kind">{ref?.evidence_type ?? 'ref'}</span>
																		<span class="src-loc">{ref?.reference ?? ref?.source_id ?? '—'}</span>
																	</li>
																{/each}
															</ul>
														{/if}
														{#if arr(block.discriminators).length}
															<button
																type="button"
																class="disc-toggle"
																onclick={() => (showDiscriminators = !showDiscriminators)}
															>
																{showDiscriminators ? 'Hide' : 'Show'} retrieval discriminators ({arr(
																	block.discriminators
																).length})
															</button>
															{#if showDiscriminators}
																<div class="disc-wrap">
																	{#each arr(block.discriminators) as d}
																		<div class="disc">
																			{#if d?.intent}<div class="disc-intent">{d.intent}</div>{/if}
																			{#if arr(d?.domain).length}
																				<div class="chips">
																					{#each arr(d.domain) as dom}<span class="kw">{dom}</span>{/each}
																				</div>
																			{/if}
																			{#each arr(d?.discriminators) as dd}
																				<div class="disc-item">
																					<span class="disc-cat">{dd?.category ?? '—'}</span>
																					<span class="disc-val">{dd?.value ?? ''}</span>
																					{#if dd?.confidence != null}
																						<span class="disc-conf mono"
																							>{Math.round((Number(dd.confidence) || 0) * 100)}%</span
																						>
																					{/if}
																				</div>
																			{/each}
																		</div>
																	{/each}
																</div>
															{/if}
														{/if}
													</section>
												{/if}

												<footer class="scene-foot mono">
													{#if block.scene_id}<span>scene_id: {block.scene_id}</span>{/if}
													{#if block.event_id}<span>event: {block.event_id}</span>{/if}
													{#if block.model_name}<span>{block.model_name}</span>{/if}
												</footer>
											</div>
										{/if}
									</div>
								{/each}
							{/if}
						</div>
					</div>

					<button
						type="button"
						class="resize-handle"
						class:active={resizing}
						aria-label="Resize the scene block panel"
						onpointerdown={startResize}
						onkeydown={onResizerKeydown}
					>
						<span class="resize-grip" aria-hidden="true"></span>
					</button>

					<div class="pdf-card">
						<div class="pdf-head">
							<div class="eyebrow">Source document</div>
							{#if activeBlock}
								<div class="pdf-ctx" title={activeBlock.title}>{activeBlock.title}</div>
							{/if}
						</div>
						{#if viewerInputId && viewerIsPdf}
							<PdfViewWindow
								inputId={viewerInputId}
								fileUrl={viewerFileUrl}
								bind:page={docPage}
								bind:zoom={pdfZoom}
								bind:numPages={pdfNumPages}
								{darkMode}
								sidebarTitle="Scene context"
								sidebarSettingsKey="scene-blocks-pdf-sidebar"
								sidebarWidthSettingLabel="Panel width"
							>
								{#snippet sidebar()}
									{#if activeBlock}
										<div class="sb">
											{#if activeBlock.scene_type?.trim()}
												<span class="scene-type">{activeBlock.scene_type}</span>
											{/if}
											<div class="sb-title">{activeBlock.title}</div>
											{#if activeBlock.summary?.trim()}
												<p class="sb-summary">{activeBlock.summary}</p>
											{/if}
											{#if arr(activeBlock.source_refs).length}
												<div class="sb-label">Source evidence</div>
												<ul class="srcrefs">
													{#each arr(activeBlock.source_refs) as ref}
														<li>
															<span class="src-kind">{ref?.evidence_type ?? 'ref'}</span>
															<span class="src-loc">{ref?.reference ?? ref?.source_id ?? '—'}</span>
														</li>
													{/each}
												</ul>
											{/if}
											<p class="sb-note">
												Scene blocks don't carry page coordinates, so use the evidence
												references above to locate the scene in the document.
											</p>
										</div>
									{:else}
										<p class="sb-note">
											Expand a scene block to see its summary and source evidence here.
										</p>
									{/if}
								{/snippet}
							</PdfViewWindow>
						{:else if viewerInputId}
							<div class="empty-state pdf-empty">
								<div class="empty-title">Source isn't a PDF</div>
								<div class="empty-copy">
									This record's source file can't be previewed here. Scene block detail is
									available in the panel on the left.
								</div>
							</div>
						{:else}
							<div class="empty-state pdf-empty">
								<div class="empty-copy">Select a record to preview its source document.</div>
							</div>
						{/if}
					</div>
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.scene-shell {
		position: relative;
		display: flex;
		height: 100%;
		flex-direction: column;
		color: var(--text);
		padding: 1rem;
		gap: 1rem;
	}
	.scene-shell.resizing {
		cursor: col-resize;
		user-select: none;
	}

	.hero {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 1.5rem;
		padding: 1.1rem 1.25rem;
		border-radius: 24px;
		border: 1px solid var(--border);
		background:
			radial-gradient(circle at top right, rgba(34, 197, 94, 0.13), transparent 44%),
			linear-gradient(180deg, rgba(15, 23, 42, 0.82), rgba(15, 23, 42, 0.62));
	}
	.hero-copy {
		min-width: 0;
	}
	.eyebrow {
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--muted);
	}
	h2 {
		margin: 0.3rem 0 0;
		font-size: 1.5rem;
		font-weight: 700;
		letter-spacing: -0.01em;
	}
	.hero p {
		margin: 0.45rem 0 0;
		max-width: 56ch;
		font-size: 0.92rem;
		line-height: 1.6;
		color: var(--muted);
	}
	.hero-stat {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		flex-shrink: 0;
		white-space: nowrap;
		font-size: 0.82rem;
		color: var(--muted);
	}
	.hero-stat-count {
		font-size: 1.55rem;
		font-weight: 800;
		line-height: 1;
		color: var(--text);
	}
	.hero-stat-label {
		font-weight: 600;
	}
	.hero-stat-sep {
		opacity: 0.5;
	}
	.hero-stat-conf {
		font-variant-numeric: tabular-nums;
	}

	.workspace {
		display: grid;
		min-height: 0;
		flex: 1;
		grid-template-columns: auto minmax(0, 1fr);
		grid-template-rows: minmax(0, 1fr);
		gap: 1rem;
	}

	.right-panel {
		display: flex;
		min-height: 0;
		flex-direction: column;
		border-radius: 24px;
		border: 1px solid var(--border);
		background: var(--panel);
		padding: 1rem;
	}
	.right-tabs {
		display: flex;
		flex-wrap: wrap;
		gap: 0.55rem;
		margin-bottom: 1rem;
	}
	.tab {
		border-radius: 14px;
		padding: 0.6rem 0.9rem;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: rgba(15, 23, 42, 0.3);
		font-size: 0.85rem;
		font-weight: 700;
		max-width: 30ch;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.tab.active {
		border-color: rgba(34, 197, 94, 0.36);
		background: rgba(34, 197, 94, 0.14);
		color: #a7f3d0;
	}
	.tab.passive {
		color: var(--muted);
		font-weight: 600;
	}

	.detail-grid {
		display: flex;
		min-height: 0;
		flex: 1;
		align-items: stretch;
		gap: 0;
	}
	.scene-card,
	.pdf-card {
		display: flex;
		flex-direction: column;
		min-height: 0;
		border-radius: 20px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: rgba(15, 23, 42, 0.26);
	}
	.scene-card {
		flex: 0 0 auto;
		overflow: hidden;
	}
	.pdf-card {
		flex: 1 1 0;
		min-width: 0;
		padding: 1rem;
	}
	.scene-card-head,
	.pdf-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 1rem;
		padding: 1rem 1.1rem;
		border-bottom: 1px solid rgba(148, 163, 184, 0.1);
	}
	.pdf-head {
		padding: 0 0 0.85rem;
	}
	.scene-card-count {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.7rem;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--muted);
	}
	.pdf-ctx {
		font-size: 0.8rem;
		color: var(--muted);
		max-width: 32ch;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.scene-list {
		display: flex;
		flex-direction: column;
		min-height: 0;
		flex: 1;
		overflow-y: auto;
		padding: 0.9rem;
		gap: 0.65rem;
	}

	.scene-block {
		flex: 0 0 auto;
		border: 1px solid rgba(148, 163, 184, 0.13);
		border-radius: 16px;
		background: rgba(2, 6, 23, 0.28);
		overflow: hidden;
		transition: border-color 160ms ease, background 160ms ease;
	}
	.scene-block.open {
		border-color: color-mix(in srgb, var(--accent) 55%, transparent);
		background: color-mix(in srgb, var(--accent) 7%, rgba(2, 6, 23, 0.3));
		box-shadow: 0 14px 34px rgba(2, 6, 23, 0.32);
	}
	.scene-row {
		display: grid;
		grid-template-columns: auto auto minmax(0, 1fr) auto;
		align-items: start;
		gap: 0.7rem;
		width: 100%;
		text-align: left;
		padding: 0.85rem 0.95rem;
		background: transparent;
		border: 0;
		cursor: pointer;
		color: inherit;
	}
	.scene-row:hover {
		background: rgba(148, 163, 184, 0.06);
	}
	.row-index {
		font-size: 0.74rem;
		font-weight: 700;
		color: var(--muted);
		padding-top: 0.15rem;
	}
	.mono {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
	}
	.chevron {
		display: inline-flex;
		color: var(--muted);
		padding-top: 0.15rem;
		transition: transform 160ms ease, color 160ms ease;
	}
	.chevron.open {
		transform: rotate(90deg);
		color: var(--accent);
	}
	.row-main {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		min-width: 0;
	}
	.row-top {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	.scene-type {
		display: inline-flex;
		align-items: center;
		padding: 0.16rem 0.5rem;
		border-radius: 999px;
		font-size: 0.66rem;
		font-weight: 700;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: #a7f3d0;
		background: rgba(34, 197, 94, 0.12);
		border: 1px solid rgba(34, 197, 94, 0.25);
	}
	.row-title {
		font-size: 0.95rem;
		font-weight: 700;
		color: var(--text);
		line-height: 1.35;
	}
	.row-summary {
		font-size: 0.84rem;
		line-height: 1.5;
		color: var(--muted);
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.scene-block.open .row-summary {
		-webkit-line-clamp: unset;
		line-clamp: unset;
	}
	.row-keywords {
		display: flex;
		flex-wrap: wrap;
		gap: 0.3rem;
		margin-top: 0.1rem;
	}
	.kw {
		font-size: 0.68rem;
		padding: 0.12rem 0.42rem;
		border-radius: 6px;
		color: var(--muted);
		background: rgba(148, 163, 184, 0.1);
	}
	.row-conf {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 0.3rem;
		padding-top: 0.15rem;
	}
	.conf-meter {
		width: 52px;
		height: 4px;
		border-radius: 999px;
		background: rgba(148, 163, 184, 0.18);
		overflow: hidden;
	}
	.conf-fill {
		display: block;
		height: 100%;
		border-radius: 999px;
	}
	.conf-high .conf-fill {
		background: var(--accent);
	}
	.conf-mid .conf-fill {
		background: #d6a93c;
	}
	.conf-low .conf-fill {
		background: #b9657a;
	}
	.conf-num {
		font-size: 0.68rem;
		color: var(--muted);
		font-variant-numeric: tabular-nums;
	}

	.scene-detail {
		padding: 0.2rem 1rem 1.05rem;
		display: flex;
		flex-direction: column;
		gap: 1.05rem;
	}
	.grp {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.grp-head {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		font-size: 0.7rem;
		font-weight: 800;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--accent);
		padding-bottom: 0.3rem;
		border-bottom: 1px solid rgba(148, 163, 184, 0.12);
	}
	.grp-sub {
		font-size: 0.68rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--muted);
		margin-top: 0.2rem;
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
	}
	.entity {
		display: inline-flex;
		align-items: baseline;
		gap: 0.4rem;
		padding: 0.28rem 0.55rem;
		border-radius: 8px;
		background: rgba(148, 163, 184, 0.1);
		border: 1px solid rgba(148, 163, 184, 0.14);
		font-size: 0.82rem;
		color: var(--text);
	}
	.entity-type {
		font-size: 0.62rem;
		font-weight: 700;
		letter-spacing: 0.05em;
		text-transform: uppercase;
		color: var(--muted);
	}
	.lines {
		margin: 0;
		padding: 0;
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}
	.lines li {
		position: relative;
		padding-left: 1rem;
		font-size: 0.86rem;
		line-height: 1.55;
		color: var(--text);
	}
	.lines li::before {
		content: '';
		position: absolute;
		left: 0.1rem;
		top: 0.62rem;
		width: 4px;
		height: 4px;
		border-radius: 50%;
		background: color-mix(in srgb, var(--accent) 70%, transparent);
	}
	.actions {
		margin: 0;
		padding: 0;
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	.actions li {
		display: flex;
		gap: 0.6rem;
		align-items: baseline;
	}
	.act-seq {
		font-size: 0.7rem;
		font-weight: 700;
		color: var(--accent);
		padding: 0.06rem 0.36rem;
		border-radius: 6px;
		background: rgba(34, 197, 94, 0.12);
		flex-shrink: 0;
	}
	.act-body {
		font-size: 0.86rem;
		line-height: 1.5;
		color: var(--text);
	}
	.act-actor {
		font-weight: 700;
		color: var(--text);
	}
	.act-actor::after {
		content: ':';
		margin-right: 0.3rem;
		color: var(--muted);
	}
	.rels {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
	}
	.rel {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.26rem 0.55rem;
		border-radius: 8px;
		background: rgba(148, 163, 184, 0.08);
		border: 1px solid rgba(148, 163, 184, 0.14);
		font-size: 0.8rem;
	}
	.rel-type {
		color: var(--muted);
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
	.rel-arrow {
		color: var(--accent);
	}
	.rel-target {
		color: var(--text);
	}
	.srcrefs {
		margin: 0;
		padding: 0;
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}
	.srcrefs li {
		display: flex;
		gap: 0.55rem;
		align-items: baseline;
		font-size: 0.82rem;
	}
	.src-kind {
		font-size: 0.62rem;
		font-weight: 700;
		letter-spacing: 0.05em;
		text-transform: uppercase;
		color: var(--muted);
		background: rgba(148, 163, 184, 0.1);
		padding: 0.12rem 0.4rem;
		border-radius: 6px;
		flex-shrink: 0;
	}
	.src-loc {
		color: var(--text);
		word-break: break-word;
	}
	.disc-toggle {
		align-self: flex-start;
		margin-top: 0.2rem;
		font-size: 0.78rem;
		font-weight: 600;
		color: var(--accent);
		background: transparent;
		border: 0;
		padding: 0.2rem 0;
		cursor: pointer;
	}
	.disc-toggle:hover {
		text-decoration: underline;
	}
	.disc-wrap {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
	.disc {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		padding: 0.65rem 0.75rem;
		border-radius: 10px;
		background: rgba(15, 23, 42, 0.4);
		border: 1px solid rgba(148, 163, 184, 0.12);
	}
	.disc-intent {
		font-size: 0.84rem;
		font-weight: 600;
		color: var(--text);
	}
	.disc-item {
		display: flex;
		gap: 0.5rem;
		align-items: baseline;
		font-size: 0.8rem;
	}
	.disc-cat {
		font-size: 0.62rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--muted);
		flex-shrink: 0;
	}
	.disc-val {
		color: var(--text);
	}
	.disc-conf {
		margin-left: auto;
		color: var(--muted);
		font-size: 0.72rem;
	}
	.scene-foot {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem 1rem;
		padding-top: 0.7rem;
		border-top: 1px solid rgba(148, 163, 184, 0.1);
		font-size: 0.7rem;
		color: var(--muted);
	}

	.resize-handle {
		flex: 0 0 16px;
		position: relative;
		padding: 0;
		border: 0;
		background: transparent;
		cursor: col-resize;
		outline: none;
		align-self: stretch;
	}
	.resize-handle::before {
		content: '';
		position: absolute;
		top: 0;
		bottom: 0;
		left: 7px;
		width: 1px;
		background: var(--ink-line);
		opacity: 0.75;
	}
	.resize-grip {
		position: absolute;
		top: 50%;
		left: 50%;
		width: 8px;
		height: 52px;
		transform: translate(-50%, -50%);
		border-radius: 999px;
		background:
			radial-gradient(circle, var(--muted) 22%, transparent 24%) center 6px / 6px 12px repeat-y,
			var(--panel-bg);
		border: 1px solid var(--ink-line-soft);
		box-shadow: 0 0 0 2px rgba(0, 0, 0, 0.14);
	}
	.resize-handle.active .resize-grip,
	.resize-handle:hover .resize-grip,
	.resize-handle:focus-visible .resize-grip {
		border-color: var(--accent);
	}

	.empty-state {
		display: grid;
		place-items: center;
		align-content: center;
		flex: 1;
		min-height: 220px;
		text-align: center;
		padding: 2rem;
		color: var(--muted);
		gap: 0.5rem;
	}
	.empty-title {
		font-size: 1rem;
		font-weight: 700;
		color: var(--text);
	}
	.empty-copy {
		font-size: 0.86rem;
		line-height: 1.6;
		max-width: 42ch;
	}
	.empty-copy code {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.82em;
		padding: 0.1rem 0.3rem;
		border-radius: 5px;
		background: rgba(148, 163, 184, 0.14);
	}
	.error-card {
		margin: 0.4rem;
		border-radius: 14px;
		border: 1px solid rgba(248, 113, 113, 0.28);
		background: rgba(127, 29, 29, 0.16);
		padding: 0.95rem 1.05rem;
	}
	.error-title {
		font-weight: 700;
		color: #fecaca;
		margin-bottom: 0.3rem;
	}
	.error-copy {
		font-size: 0.85rem;
		line-height: 1.5;
		color: #fca5a5;
	}

	.skeleton-row {
		flex: 0 0 auto;
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
		padding: 0.95rem;
		border-radius: 16px;
		border: 1px solid rgba(148, 163, 184, 0.1);
		background: rgba(2, 6, 23, 0.28);
		animation: pulse 1.5s ease-in-out infinite;
	}
	.sk {
		border-radius: 6px;
		background: rgba(148, 163, 184, 0.16);
	}
	.sk-pill {
		width: 78px;
		height: 14px;
	}
	.sk-title {
		width: 70%;
		height: 13px;
	}
	.sk-line {
		width: 92%;
		height: 10px;
	}
	@keyframes pulse {
		0%,
		100% {
			opacity: 1;
		}
		50% {
			opacity: 0.45;
		}
	}

	.sb {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
	.sb-title {
		font-size: 0.95rem;
		font-weight: 700;
		color: var(--text);
		line-height: 1.4;
	}
	.sb-summary {
		margin: 0;
		font-size: 0.84rem;
		line-height: 1.55;
		color: var(--muted);
	}
	.sb-label {
		font-size: 0.66rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--muted);
		margin-top: 0.3rem;
	}
	.sb-note {
		margin: 0.4rem 0 0;
		font-size: 0.78rem;
		line-height: 1.5;
		color: var(--muted);
		opacity: 0.85;
	}

	@media (max-width: 980px) {
		.workspace {
			grid-template-columns: minmax(0, 1fr);
		}
		.detail-grid {
			flex-direction: column;
			gap: 1rem;
		}
		.scene-card {
			width: 100% !important;
			flex: none !important;
		}
		.resize-handle {
			display: none;
		}
		.hero {
			flex-direction: column;
			align-items: flex-start;
			gap: 1rem;
		}
	}
</style>

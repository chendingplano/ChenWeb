<script lang="ts">
	import type { PdfPageViewport } from './shared-pdf-viewer.svelte';
	import {
		extractKbMetrics,
		saveExtractedKbMetrics,
		extractKbProvisions,
		saveExtractedKbProvisions,
		createKbProvision,
		updateRawLine,
		type ExtractedKbMetric,
		type ExtractedKbProvision,
		type RawLine,
		type SourceLineSpan
	} from '$lib/services/kbService';

	let {
		inputId,
		open = $bindable(false),
		rawLines,
		selectionDetail,
		onrawlineupdate
	}: {
		inputId: number | null;
		open?: boolean;
		rawLines: RawLine[];
		selectionDetail: Array<{
			pageNumber: number;
			viewportY1: number;
			viewportY2: number;
			viewport: PdfPageViewport;
		}> | null;
		onrawlineupdate?: (updated: RawLine) => void;
	} = $props();

	let bufferLines = $state<number[]>([]);
	let provisionPreview = $state<ExtractedKbProvision[]>([]);
	let translateX = $state(0);
	let translateY = $state(0);
	let isDragging = $state(false);
	let dragStartX = 0;
	let dragStartY = 0;

	$effect(() => {
		if (open) {
			translateX = 0;
			translateY = 0;
		}
	});

	function onHeaderPointerDown(e: PointerEvent) {
		isDragging = true;
		dragStartX = e.clientX - translateX;
		dragStartY = e.clientY - translateY;
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		e.preventDefault();
	}

	function onHeaderPointerMove(e: PointerEvent) {
		if (!isDragging) return;
		translateX = e.clientX - dragStartX;
		translateY = e.clientY - dragStartY;
	}

	function onHeaderPointerUp() {
		isDragging = false;
	}

	let editKey = $state<string | null>(null);
	let editContent = $state('');
	let busyAction = $state<'line' | 'extract' | 'save' | 'provision' | null>(null);
	let extractedPreview = $state<ExtractedKbMetric[]>([]);
	let busy = $derived(busyAction !== null);

	type Operation = 'extract-metrics' | 'extract-provisions' | 'write-comments' | 'ask-ai';
	let selectedOperation = $state<Operation>('extract-metrics');
	let commentText = $state('');

	let opMeta = $derived.by(() => {
		switch (selectedOperation) {
			case 'extract-metrics':
				return {
					eyebrow: 'KB.Metrics',
					title: 'Add Metric',
					subtitle:
						'Review selected lines, extract candidate metrics, remove any you do not want, then save the remaining metrics to the database.'
				};
			case 'extract-provisions':
				return {
					eyebrow: 'KB.Provisions',
					title: 'Extract Provision',
					subtitle: 'Review selected lines and extract a compliance provision.'
				};
			case 'write-comments':
				return {
					eyebrow: 'KB.Comments',
					title: 'Write Comment',
					subtitle: 'Review the selected lines and write a comment about the selected content.'
				};
			case 'ask-ai':
				return {
					eyebrow: 'KB.AI',
					title: 'Ask AI',
					subtitle: 'Ask AI about the selected content. Coming soon.'
				};
		}
	});

	$effect(() => {
		if (!selectionDetail || selectionDetail.length === 0 || rawLines.length === 0) return;
		const selected: number[] = [];
		for (const { pageNumber, viewportY1, viewportY2, viewport } of selectionDetail) {
			const pageLines = rawLines.filter((l) => l.page_number === pageNumber);
			for (const line of pageLines) {
				if (!Array.isArray(line.coords) || line.coords.length < 4) continue;
				const [, vy1, , vy2] = viewport.convertToViewportRectangle(line.coords.slice(0, 4));
				const lineTop = Math.min(vy1, vy2);
				const lineBottom = Math.max(vy1, vy2);
				if (Math.max(lineTop, viewportY1) <= Math.min(lineBottom, viewportY2)) {
					selected.push(line.line_number);
				}
			}
		}
		bufferLines = selected;
		extractedPreview = [];
		editKey = null;
		editContent = '';
		busyAction = null;
	});

	let dialogLines = $derived.by(() => {
		const seen = new Set<string>();
		const result: Array<RawLine & { key: string }> = [];
		for (const lineNo of bufferLines) {
			for (const ln of rawLines) {
				if (ln.line_number !== lineNo) continue;
				const key = `${ln.page_number}:${ln.line_number}`;
				if (seen.has(key)) continue;
				seen.add(key);
				result.push({ ...ln, key });
			}
		}
		return result.sort((a, b) => a.line_number - b.line_number);
	});

	let canAddPrevious = $derived.by(() => {
		if (bufferLines.length === 0 || rawLines.length === 0) return false;
		const minLine = Math.min(...bufferLines);
		return rawLines.some((ln) => ln.line_number < minLine);
	});
	let canAddNext = $derived.by(() => {
		if (bufferLines.length === 0 || rawLines.length === 0) return false;
		const maxLine = Math.max(...bufferLines);
		return rawLines.some((ln) => ln.line_number > maxLine);
	});

	function close() {
		open = false;
		editKey = null;
		editContent = '';
		bufferLines = [];
		busyAction = null;
		extractedPreview = [];
		provisionPreview = [];
		commentText = '';
	}

	function startEditLine(key: string, content: string) {
		editKey = key;
		editContent = content;
	}
	function cancelEditLine() {
		editKey = null;
		editContent = '';
	}

	async function saveEditLine(pageNo: number, lineNo: number) {
		if (!inputId || !editKey) return;
		const confirmed = window.confirm('Save changes to the original file?');
		if (!confirmed) return;
		busyAction = 'line';
		try {
			const res = await updateRawLine({
				input_record_id: inputId,
				page_number: pageNo,
				line_number: lineNo,
				content: editContent
			});
			onrawlineupdate?.(res.line);
			editKey = null;
			editContent = '';
			bufferLines = [];
			extractedPreview = [];
		} catch (err) {
			alert(err instanceof Error ? err.message : 'Failed to save line');
		} finally {
			busyAction = null;
		}
	}

	function deleteLine(key: string) {
		const removedLines: number[] = [];
		for (const ln of dialogLines) {
			if (ln.key === key) removedLines.push(ln.line_number);
		}
		bufferLines = bufferLines.filter((ln) => !removedLines.includes(ln));
		extractedPreview = [];
	}

	function addPreviousLine() {
		if (bufferLines.length === 0) return;
		const minLine = Math.min(...bufferLines);
		const prev = rawLines
			.filter((ln) => ln.line_number < minLine && !bufferLines.includes(ln.line_number))
			.sort((a, b) => b.line_number - a.line_number)[0];
		if (prev) {
			bufferLines = [prev.line_number, ...bufferLines];
			extractedPreview = [];
		}
	}

	function addNextLine() {
		if (bufferLines.length === 0) return;
		const maxLine = Math.max(...bufferLines);
		const next = rawLines
			.filter((ln) => ln.line_number > maxLine && !bufferLines.includes(ln.line_number))
			.sort((a, b) => a.line_number - b.line_number)[0];
		if (next) {
			bufferLines = [...bufferLines, next.line_number];
			extractedPreview = [];
		}
	}

	async function extractMetric() {
		if (!inputId || dialogLines.length === 0) return;
		busyAction = 'extract';
		try {
			const nums = [...new Set(dialogLines.map((l) => l.line_number))].sort((a, b) => a - b);
			const lineSpecs: string[] = [];
			let i = 0;
			while (i < nums.length) {
				let j = i;
				while (j + 1 < nums.length && nums[j + 1] === nums[j] + 1) j++;
				lineSpecs.push(i === j ? `${nums[i]}` : `${nums[i]}-${nums[j]}`);
				i = j + 1;
			}
			const result = await extractKbMetrics({ record_id: inputId, lines: lineSpecs });
			extractedPreview = result.metrics ?? [];
		} catch (err) {
			alert(err instanceof Error ? err.message : 'Failed to extract metrics');
		} finally {
			busyAction = null;
		}
	}

	async function saveExtracted() {
		if (!inputId || extractedPreview.length === 0) return;
		busyAction = 'save';
		try {
			await saveExtractedKbMetrics({ record_id: inputId, metrics: extractedPreview });
			close();
		} catch (err) {
			alert(err instanceof Error ? err.message : 'Failed to save metrics');
		} finally {
			busyAction = null;
		}
	}

	async function extractProvision() {
		if (!inputId || dialogLines.length === 0) return;
		busyAction = 'extract';
		try {
			const spans = dialogLines.map((l) => ({
				page_number: l.page_number,
				line_number: l.line_number
			}));
			const result = await extractKbProvisions({ record_id: inputId, source_line_spans: spans });
			provisionPreview = result.provisions ?? [];
		} catch (err) {
			alert(err instanceof Error ? err.message : 'Failed to extract provisions');
		} finally {
			busyAction = null;
		}
	}

	async function saveProvisions() {
		if (!inputId || provisionPreview.length === 0) return;
		busyAction = 'save';
		try {
			await saveExtractedKbProvisions({ record_id: inputId, provisions: provisionPreview });
			close();
		} catch (err) {
			alert(err instanceof Error ? err.message : 'Failed to save provisions');
		} finally {
			busyAction = null;
		}
	}

	function removeProvisionPreview(index: number) {
		provisionPreview = provisionPreview.filter((_, idx) => idx !== index);
	}

	function provisionDisplayName(p: ExtractedKbProvision, index: number): string {
		return p.prov_name?.trim() || p.provision_subject?.trim() || `Provision ${index + 1}`;
	}

	function saveComment() {
		// TODO: connect to backend
		alert('Write Comments is not yet connected to the backend.');
	}

	function runOperation() {
		if (selectedOperation === 'extract-metrics') extractMetric();
		else if (selectedOperation === 'extract-provisions') extractProvision();
	}

	function removePreview(index: number) {
		extractedPreview = extractedPreview.filter((_, idx) => idx !== index);
	}

	function previewName(m: ExtractedKbMetric, index: number): string {
		return m.metric_name?.trim() || m.metric_subject?.trim() || `Metric ${index + 1}`;
	}

	function confidencePct(c?: number): string {
		if (c == null) return '—';
		return `${Math.round(c * 100)}%`;
	}
</script>

{#if open}
	<div class="dialog-overlay" aria-hidden="true">
		<div
			class="dialog am-dialog"
			role="dialog"
			aria-modal="true"
			aria-label="Add metric"
			tabindex="0"
			onkeydown={(e) => {
				if (e.key === 'Escape') close();
			}}
			style="transform: translate({translateX}px, {translateY}px)"
		>
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div
				class="dialog-head"
				onpointerdown={onHeaderPointerDown}
				onpointermove={onHeaderPointerMove}
				onpointerup={onHeaderPointerUp}
				style="cursor: {isDragging ? 'grabbing' : 'grab'}"
			>
				<div>
					<div class="dialog-eyebrow">{opMeta.eyebrow}</div>
					<h2 class="dialog-title">{opMeta.title}</h2>
					<p class="dialog-subtitle">{opMeta.subtitle}</p>
				</div>
			</div>

			<div class="dialog-controls am-body">
				{#if dialogLines.length === 0}
					<div class="dialog-section am-empty-section">
						<div class="empty-glyph">§</div>
						<div class="empty-title">No lines selected</div>
						<div class="empty-sub">
							Drag to select lines on the PDF, then open this dialog again.
						</div>
					</div>
				{:else}
					<div class="dialog-section">
						<div class="dialog-section-head">
							<span class="dialog-section-title">Selected Lines</span>
							<span class="dialog-section-copy"
								>{dialogLines.length} line{dialogLines.length === 1 ? '' : 's'} selected</span
							>
							<button
								type="button"
								class="am-btn am-btn-head-add"
								disabled={!canAddPrevious}
								onclick={addPreviousLine}>+ Add</button
							>
						</div>
						<div class="am-table-wrap">
							<table class="am-table">
								<thead>
									<tr>
										<th class="am-col-line">Line #</th>
										<th class="am-col-page">Page</th>
										<th class="am-col-type">Type</th>
										<th class="am-col-content">Content</th>
										<th class="am-col-actions"></th>
									</tr>
								</thead>
								<tbody>
									{#each dialogLines as line (line.key)}
										<tr class="am-row">
											<td class="am-mono">{line.line_number}</td>
											<td class="am-mono">{line.page_number}</td>
											<td><span class="am-type-badge">{line.line_type}</span></td>
											<td class="am-content-cell">
												{#if editKey === line.key}
													<input
														class="am-edit-input"
														type="text"
														bind:value={editContent}
														onkeydown={(e) => {
															if (e.key === 'Escape') cancelEditLine();
															if (e.key === 'Enter')
																saveEditLine(line.page_number, line.line_number);
														}}
													/>
												{:else}
													<span class="am-content-text">{line.content}</span>
												{/if}
											</td>
											<td class="am-action-cell">
												{#if editKey === line.key}
													<button
														type="button"
														class="am-btn am-btn-save"
														disabled={busy}
														onclick={() => saveEditLine(line.page_number, line.line_number)}
														>Save</button
													>
													<button
														type="button"
														class="am-btn am-btn-cancel-row"
														onclick={cancelEditLine}>Cancel</button
													>
												{:else}
													<button
														type="button"
														class="am-btn am-btn-edit"
														onclick={() => startEditLine(line.key, line.content)}>Edit</button
													>
													<button
														type="button"
														class="am-btn am-btn-delete"
														onclick={() => deleteLine(line.key)}>Remove</button
													>
												{/if}
											</td>
										</tr>
									{/each}
								</tbody>
							</table>
							<div class="am-table-foot">
								<button
									type="button"
									class="am-btn am-btn-foot-add"
									disabled={!canAddNext}
									onclick={addNextLine}>+ Add</button
								>
							</div>
						</div>
					</div>

					{#if selectedOperation === 'extract-metrics'}
					<div class="dialog-section">
						<div class="dialog-section-head">
							<span class="dialog-section-title">Extracted Metrics</span>
							<span class="dialog-section-copy"
								>{extractedPreview.length} metric{extractedPreview.length === 1
									? ''
									: 's'} ready</span
							>
						</div>
						{#if busyAction === 'extract'}
							<div class="am-status-row" aria-live="polite">
								<span class="am-spinner" aria-hidden="true"></span>
								<span>Extracting metrics from the selected lines…</span>
							</div>
						{:else if extractedPreview.length === 0}
							<div class="metadata-empty">
								Press <strong>Extract Metric</strong> to preview the metrics returned by the backend.
							</div>
						{:else}
							<div class="am-preview-list">
								{#each extractedPreview as metric, idx (`${previewName(metric, idx)}-${idx}`)}
									<div class="am-preview-card">
										<div class="am-preview-head">
											<div>
												<div class="am-preview-name">{previewName(metric, idx)}</div>
												<div class="am-preview-meta">
													<span>{confidencePct(metric.confidence)}</span>
													{#if metric.location_type}<span>{metric.location_type}</span>{/if}
													{#if metric.metric_unit}<span>{metric.metric_unit}</span>{/if}
												</div>
											</div>
											<button
												type="button"
												class="am-btn am-btn-delete"
												disabled={busy}
												onclick={() => removePreview(idx)}>Remove</button
											>
										</div>
										{#if metric.metric_desc}
											<div class="am-preview-desc">{metric.metric_desc}</div>
										{/if}
										<div class="am-preview-fields">
											{#if metric.metric_value}<span class="chip chip-mono"
													>{metric.metric_value}</span
												>{/if}
											{#if metric.value_data_type}<span class="chip chip-quiet"
													>{metric.value_data_type}</span
												>{/if}
											{#if metric.table_name_or_section}<span class="chip"
													>{metric.table_name_or_section}</span
												>{/if}
										</div>
									</div>
								{/each}
							</div>
						{/if}
					</div>
				{/if}
					{#if selectedOperation === 'extract-provisions'}
					<div class="dialog-section">
						<div class="dialog-section-head">
							<span class="dialog-section-title">Extracted Provisions</span>
							<span class="dialog-section-copy"
								>{provisionPreview.length} provision{provisionPreview.length === 1
									? ''
									: 's'} ready</span
							>
						</div>
						{#if busyAction === 'extract'}
							<div class="am-status-row" aria-live="polite">
								<span class="am-spinner" aria-hidden="true"></span>
								<span>Extracting provisions from the selected lines…</span>
							</div>
						{:else if provisionPreview.length === 0}
							<div class="metadata-empty">
								Press <strong>Run</strong> to preview the provisions returned by the AI.
							</div>
						{:else}
							<div class="am-preview-list">
								{#each provisionPreview as prov, idx (`${provisionDisplayName(prov, idx)}-${idx}`)}
									<div class="am-preview-card">
										<div class="am-preview-head">
											<div>
												<div class="am-preview-name">{provisionDisplayName(prov, idx)}</div>
												<div class="am-preview-meta">
													<span>{confidencePct(prov.confidence)}</span>
													{#if prov.provision_type}<span>{prov.provision_type}</span>{/if}
													{#if prov.location_type}<span>{prov.location_type}</span>{/if}
												</div>
											</div>
											<button
												type="button"
												class="am-btn am-btn-delete"
												disabled={busy}
												onclick={() => removeProvisionPreview(idx)}>Remove</button
											>
										</div>
										{#if prov.prov_desc || prov.provision_en}
											<div class="am-preview-desc">{prov.prov_desc || prov.provision_en}</div>
										{/if}
									</div>
								{/each}
							</div>
						{/if}
					</div>
					{/if}
					{#if selectedOperation === 'write-comments'}
					<div class="dialog-section">
						<div class="dialog-section-head">
							<span class="dialog-section-title">Comment</span>
						</div>
						<textarea
							class="am-comment-textarea"
							placeholder="Write your comment about the selected lines…"
							bind:value={commentText}
						></textarea>
					</div>
					{/if}
			{/if}
			</div>

			<div class="dialog-foot">
				<div class="dialog-foot-left">
					<button
						type="button"
						class="am-btn-foot am-btn-help"
						onclick={() => {
							alert(
								'Select PDF lines and choose an operation.\n\n' +
									'• Drag on the PDF to select lines\n' +
									'• Edit: modify line content (saves to the original file)\n' +
									'• Remove: remove a line from this selection\n' +
									'• Extract Provisions: create a new provision from the remaining lines\n' +
									'• Extract Metrics: preview metrics returned from the backend, then Save\n' +
									'• Write Comments: write a comment about the selected content'
							);
						}}>Help</button
					>
					<div class="dialog-operation-select">
						<label class="dialog-operation-label" for="op-select">Operation</label>
						<select
							id="op-select"
							class="dialog-operation-dropdown"
							bind:value={selectedOperation}
							onchange={() => {
								extractedPreview = [];
								provisionPreview = [];
								commentText = '';
							}}
						>
							<option value="extract-metrics">Extract Metrics</option>
							<option value="extract-provisions">Extract Provisions</option>
							<option value="write-comments">Write Comments</option>
							<option value="ask-ai" disabled>Ask AI (coming soon)</option>
						</select>
					</div>
					<button
						type="button"
						class="am-btn-foot am-btn-run dialog-search-btn"
						disabled={selectedOperation !== 'extract-metrics' &&
							selectedOperation !== 'extract-provisions'}
						onclick={runOperation}
					>
						{#if busyAction === 'extract'}
							<span class="am-btn-inline"
								><span class="am-spinner am-spinner-dark" aria-hidden="true"></span>Running…</span
							>
						{:else}
							Run
						{/if}
					</button>
				</div>
				<div class="dialog-foot-buttons">
					<button type="button" class="am-btn-foot am-btn-foot-cancel" onclick={close}
						>Close</button
					>
					{#if selectedOperation === 'extract-metrics'}
						<button
							type="button"
							class="am-btn-foot am-btn-foot-save dialog-search-btn"
							disabled={extractedPreview.length === 0 || busy}
							onclick={saveExtracted}
						>
							{#if busyAction === 'save'}
								<span class="am-btn-inline"
									><span class="am-spinner am-spinner-dark" aria-hidden="true"></span>Saving…</span
								>
							{:else}
								Save
							{/if}
						</button>
					{:else if selectedOperation === 'extract-provisions'}
						<button
							type="button"
							class="am-btn-foot am-btn-foot-save dialog-search-btn"
							disabled={provisionPreview.length === 0 || busy}
							onclick={saveProvisions}
						>
							{#if busyAction === 'save'}
								<span class="am-btn-inline"
									><span class="am-spinner am-spinner-dark" aria-hidden="true"></span>Saving…</span
								>
							{:else}
								Save
							{/if}
						</button>
					{:else if selectedOperation === 'write-comments'}
						<button
							type="button"
							class="am-btn-foot am-btn-foot-save dialog-search-btn"
							disabled={commentText.trim().length === 0 || busy}
							onclick={saveComment}>Save Comment</button
						>
					{/if}
				</div>
			</div>
		</div>
	</div>
{/if}

<style>
	@import url('https://fonts.googleapis.com/css2?family=Cormorant+Garamond:ital,wght@0,400;0,500;0,600;1,400&family=JetBrains+Mono:wght@400;500;600&family=Inter+Tight:wght@400;500;600&display=swap');

	/* ---- Tokens ---- */
	.dialog-overlay {
		--brass: #d4a24c;
		--teal: #5dafa8;
		--crimson: #c8553d;
		--text-primary: #ede7d3;
		--text-secondary: #b5ae94;
		--text-muted: #7c7560;
		--ink-line: #2a3140;
		--ink-line-soft: #1f2530;
		--font-serif: 'Cormorant Garamond', 'Playfair Display', Georgia, serif;
		--font-mono: 'JetBrains Mono', 'IBM Plex Mono', monospace;
		--font-sans: 'Inter Tight', system-ui, sans-serif;
	}

	/* ---- Overlay ---- */
	.dialog-overlay {
		position: fixed;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 50;
		padding: 24px;
		pointer-events: none;
	}

	/* ---- Dialog shell ---- */
	.dialog {
		width: 100%;
		max-width: 1210px;
		max-height: min(90vh, 980px);
		display: flex;
		flex-direction: column;
		border-radius: 24px;
		overflow: auto;
		background: #111827;
		color: #f3eedf;
		border: 1px solid rgba(148, 163, 184, 0.16);
		box-shadow:
			0 30px 80px rgba(0, 0, 0, 0.55),
			0 0 0 1px rgba(212, 162, 76, 0.08);
		resize: both;
		min-width: 920px;
		min-height: 720px;
		pointer-events: auto;
	}
	.am-dialog {
		min-width: 860px;
		min-height: 540px;
		max-width: 1100px;
		max-height: min(88vh, 900px);
	}

	/* ---- Header ---- */
	.dialog-head {
		padding: 24px 28px 16px;
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		border-bottom: 1px solid var(--ink-line-soft);
		background: linear-gradient(180deg, rgba(212, 162, 76, 0.09), rgba(212, 162, 76, 0));
		user-select: none;
	}
	.dialog-eyebrow {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.16em;
		color: var(--brass);
		text-transform: uppercase;
		margin-bottom: 4px;
	}
	.dialog-title {
		font-family: var(--font-serif);
		font-size: 28px;
		font-weight: 500;
		margin: 0;
		color: var(--text-primary);
	}
	.dialog-subtitle {
		margin: 8px 0 0;
		max-width: 640px;
		font-size: 13px;
		line-height: 1.45;
		color: var(--text-secondary);
	}

	/* ---- Body ---- */
	.dialog-controls {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 16px 28px 12px;
		background: linear-gradient(180deg, rgba(255, 255, 255, 0.025), rgba(0, 0, 0, 0.03));
		flex: 0 0 auto;
	}
	.am-body {
		flex: 1 1 auto;
		overflow-y: auto;
		min-height: 0;
	}
	.dialog-section {
		border: 1px solid rgba(212, 162, 76, 0.16);
		border-radius: 20px;
		padding: 14px;
		background:
			linear-gradient(180deg, rgba(255, 255, 255, 0.04), rgba(255, 255, 255, 0.01)), #171c26;
	}
	.dialog-section-head {
		display: flex;
		justify-content: space-between;
		gap: 16px;
		margin-bottom: 10px;
		align-items: baseline;
	}
	.dialog-section-title {
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--brass);
	}
	.dialog-section-copy {
		font-size: 12px;
		color: var(--text-secondary);
	}

	/* ---- Empty state ---- */
	.am-empty-section {
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: 48px 24px;
		gap: 8px;
		text-align: center;
	}
	.empty-glyph {
		font-family: var(--font-serif);
		font-size: 48px;
		color: var(--text-muted);
		line-height: 1;
	}
	.empty-title {
		font-family: var(--font-serif);
		font-size: 20px;
		color: var(--text-secondary);
	}
	.empty-sub {
		font-size: 13px;
		color: var(--text-muted);
	}

	/* ---- Table ---- */
	.am-table-wrap {
		overflow-x: auto;
		border-radius: 12px;
		background: #13192280;
		border: 1px solid rgba(255, 255, 255, 0.05);
	}
	.am-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
	}
	.am-table thead th {
		font-family: var(--font-mono);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.12em;
		color: var(--text-muted);
		text-align: left;
		padding: 10px 14px;
		border-bottom: 1px solid rgba(255, 255, 255, 0.07);
		background: #181d27;
		white-space: nowrap;
	}
	.am-table tbody tr {
		border-bottom: 1px solid rgba(255, 255, 255, 0.04);
		transition: background 100ms;
	}
	.am-table tbody tr:last-child {
		border-bottom: none;
	}
	.am-table tbody tr:hover {
		background: rgba(255, 255, 255, 0.03);
	}
	.am-table td {
		padding: 10px 14px;
		vertical-align: middle;
		color: var(--text-primary);
	}
	.am-col-line,
	.am-col-page {
		width: 64px;
	}
	.am-col-type {
		width: 130px;
	}
	.am-col-actions {
		width: 180px;
	}
	.am-mono {
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--text-muted);
	}
	.am-type-badge {
		display: inline-block;
		padding: 2px 8px;
		border-radius: 4px;
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.06em;
		text-transform: lowercase;
		background: rgba(93, 175, 168, 0.12);
		color: var(--teal);
		border: 1px solid rgba(93, 175, 168, 0.2);
		white-space: nowrap;
	}
	.am-content-cell {
		max-width: 480px;
	}
	.am-content-text {
		display: block;
		line-height: 1.45;
		white-space: pre-wrap;
		word-break: break-word;
	}
	.am-action-cell {
		display: flex;
		gap: 6px;
		align-items: center;
		white-space: nowrap;
	}
	.am-edit-input {
		width: 100%;
		min-width: 200px;
		padding: 6px 10px;
		background: #1a202b;
		border: 1px solid rgba(212, 162, 76, 0.35);
		border-radius: 8px;
		color: var(--text-primary);
		font-family: inherit;
		font-size: 13px;
		outline: none;
	}
	.am-edit-input:focus {
		border-color: var(--brass);
		box-shadow: 0 0 0 2px rgba(212, 162, 76, 0.15);
	}

	/* ---- Buttons (table) ---- */
	.am-btn {
		height: 28px;
		padding: 0 10px;
		border-radius: 6px;
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.06em;
		cursor: pointer;
		transition:
			background 120ms,
			border-color 120ms,
			color 120ms;
		white-space: nowrap;
	}
	.am-btn-edit {
		background: rgba(255, 255, 255, 0.06);
		border: 1px solid rgba(255, 255, 255, 0.14);
		color: var(--text-secondary);
	}
	.am-btn-edit:hover {
		background: rgba(255, 255, 255, 0.11);
		border-color: rgba(255, 255, 255, 0.24);
		color: var(--text-primary);
	}
	.am-btn-delete {
		background: rgba(200, 85, 61, 0.12);
		border: 1px solid rgba(200, 85, 61, 0.3);
		color: var(--crimson);
	}
	.am-btn-delete:hover {
		background: rgba(200, 85, 61, 0.22);
		border-color: rgba(200, 85, 61, 0.52);
	}
	.am-btn-save {
		background: rgba(212, 162, 76, 0.15);
		border: 1px solid rgba(212, 162, 76, 0.38);
		color: var(--brass);
	}
	.am-btn-save:hover:not(:disabled) {
		background: rgba(212, 162, 76, 0.26);
		border-color: var(--brass);
	}
	.am-btn-save:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.am-btn-cancel-row {
		background: rgba(255, 255, 255, 0.04);
		border: 1px solid rgba(255, 255, 255, 0.1);
		color: var(--text-muted);
	}
	.am-btn-cancel-row:hover {
		background: rgba(255, 255, 255, 0.08);
		color: var(--text-secondary);
	}

	/* ---- Table footer add button ---- */
	.am-table-foot {
		display: flex;
		justify-content: flex-end;
		padding: 8px 14px;
		border-top: 1px solid rgba(255, 255, 255, 0.05);
	}
	.am-btn-head-add,
	.am-btn-foot-add {
		height: 28px;
		padding: 0 12px;
		border-radius: 6px;
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.06em;
		cursor: pointer;
		transition:
			background 120ms,
			border-color 120ms,
			color 120ms;
		background: rgba(212, 162, 76, 0.1);
		border: 1px solid rgba(212, 162, 76, 0.28);
		color: var(--brass);
	}
	.am-btn-head-add:hover:not(:disabled),
	.am-btn-foot-add:hover:not(:disabled) {
		background: rgba(212, 162, 76, 0.2);
		border-color: var(--brass);
	}
	.am-btn-head-add:disabled,
	.am-btn-foot-add:disabled {
		opacity: 0.35;
		cursor: not-allowed;
	}

	/* ---- Status / spinner ---- */
	.am-status-row {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 10px 0;
		font-size: 13px;
		color: var(--text-secondary);
	}
	.am-spinner {
		display: inline-block;
		width: 14px;
		height: 14px;
		border: 2px solid rgba(212, 162, 76, 0.25);
		border-top-color: var(--brass);
		border-radius: 50%;
		animation: spin 0.7s linear infinite;
		flex-shrink: 0;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	/* ---- Preview cards ---- */
	.am-preview-list {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.am-preview-card {
		padding: 12px 14px;
		border-radius: 12px;
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid rgba(255, 255, 255, 0.07);
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.am-preview-head {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 12px;
	}
	.am-preview-name {
		font-family: var(--font-serif);
		font-size: 16px;
		color: var(--text-primary);
	}
	.am-preview-meta {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
		font-family: var(--font-mono);
		font-size: 10px;
		color: var(--text-muted);
		letter-spacing: 0.06em;
		text-transform: uppercase;
	}
	.am-preview-desc {
		font-size: 13px;
		color: var(--text-secondary);
		line-height: 1.45;
	}
	.am-preview-fields {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}

	/* ---- Chips ---- */
	.chip {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		font-family: var(--font-mono);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		padding: 3px 8px;
		background: transparent;
		border: 1px solid var(--ink-line);
		color: var(--text-secondary);
	}
	.chip-mono {
		color: var(--brass);
		border-color: var(--brass);
	}
	.chip-quiet {
		color: var(--text-muted);
	}

	/* ---- Comment textarea ---- */
	.am-comment-textarea {
		width: 100%;
		min-height: 120px;
		padding: 12px 14px;
		background: #13192280;
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: 12px;
		color: var(--text-primary);
		font-family: inherit;
		font-size: 13px;
		line-height: 1.55;
		resize: vertical;
		outline: none;
		box-sizing: border-box;
	}
	.am-comment-textarea:focus {
		border-color: rgba(212, 162, 76, 0.4);
		box-shadow: 0 0 0 2px rgba(212, 162, 76, 0.1);
	}
	.am-comment-textarea::placeholder {
		color: var(--text-muted);
	}

	/* ---- Metadata empty ---- */
	.metadata-empty {
		font-size: 12px;
		color: var(--text-muted);
	}

	/* ---- Footer buttons ---- */
	.am-btn-inline {
		display: inline-flex;
		align-items: center;
		gap: 8px;
	}
	.am-btn-foot {
		height: 40px;
		padding: 0 18px;
		border-radius: 10px;
		font-family: var(--font-mono);
		font-size: 12px;
		letter-spacing: 0.08em;
		cursor: pointer;
		transition:
			background 120ms,
			border-color 120ms,
			color 120ms;
	}
	.am-btn-help {
		background: transparent;
		border: 1px solid rgba(255, 255, 255, 0.1);
		color: var(--text-muted);
	}
	.am-btn-help:hover {
		border-color: rgba(255, 255, 255, 0.22);
		color: var(--text-secondary);
	}
	.am-btn-foot-cancel {
		background: rgba(255, 255, 255, 0.05);
		border: 1px solid rgba(255, 255, 255, 0.12);
		color: var(--text-secondary);
	}
	.am-btn-foot-cancel:hover {
		background: rgba(255, 255, 255, 0.09);
		color: var(--text-primary);
	}
	.am-btn-run {
		min-width: 72px;
	}
	.am-spinner-dark {
		border-color: rgba(21, 17, 10, 0.3);
		border-top-color: #15110a;
	}
	.dialog-search-btn {
		min-width: 140px;
		background: #d4a24c !important;
		color: #15110a !important;
		border: 1px solid #e0b768 !important;
		box-shadow: 0 8px 20px rgba(212, 162, 76, 0.22);
		opacity: 1 !important;
	}
	.dialog-search-btn:hover:not(:disabled) {
		background: #e0b768 !important;
		color: #15110a !important;
	}
	.dialog-search-btn:disabled {
		background: rgba(212, 162, 76, 0.35) !important;
		color: rgba(21, 17, 10, 0.7) !important;
	}
	.am-btn-foot-save {
		background: rgba(93, 175, 168, 0.9) !important;
		border: 1px solid rgba(93, 175, 168, 0.95) !important;
		color: #081311 !important;
		box-shadow: 0 8px 20px rgba(93, 175, 168, 0.18);
	}
	.am-btn-foot-save:hover:not(:disabled) {
		background: #74c4bd !important;
		color: #081311 !important;
	}
	.am-btn-foot-save:disabled {
		background: rgba(93, 175, 168, 0.28) !important;
		color: rgba(8, 19, 17, 0.65) !important;
	}
	.dialog-foot {
		padding: 14px 28px 18px;
		display: flex;
		justify-content: space-between;
		align-items: center;
		flex: 0 0 auto;
		background: #171c26;
	}
	.dialog-foot-left {
		display: flex;
		align-items: center;
		gap: 14px;
	}
	.dialog-operation-select {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.dialog-operation-label {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--text-muted);
		white-space: nowrap;
	}
	.dialog-operation-dropdown {
		height: 36px;
		padding: 0 28px 0 10px;
		background: rgba(255, 255, 255, 0.06);
		border: 1px solid rgba(255, 255, 255, 0.14);
		border-radius: 8px;
		color: var(--text-primary);
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.04em;
		cursor: pointer;
		appearance: none;
		-webkit-appearance: none;
		background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%237c7560' stroke-width='2'%3E%3Cpath d='M6 9l6 6 6-6'/%3E%3C/svg%3E");
		background-repeat: no-repeat;
		background-position: right 8px center;
		outline: none;
	}
	.dialog-operation-dropdown:focus {
		border-color: rgba(212, 162, 76, 0.5);
		box-shadow: 0 0 0 2px rgba(212, 162, 76, 0.12);
	}
	.dialog-operation-dropdown option {
		background: #1a202b;
		color: var(--text-primary);
	}
	.dialog-foot-buttons {
		display: flex;
		gap: 10px;
	}

	/* ---- Scrollbar ---- */
	.dialog::-webkit-scrollbar {
		width: 12px;
		height: 12px;
	}
	.dialog::-webkit-scrollbar-thumb {
		background: rgba(212, 162, 76, 0.34);
		border-radius: 999px;
		border: 2px solid transparent;
		background-clip: padding-box;
	}
	.dialog::-webkit-scrollbar-track {
		background: rgba(255, 255, 255, 0.03);
	}

	@media (max-width: 820px) {
		.dialog-overlay {
			padding: 12px;
		}
		.dialog {
			max-height: 94vh;
			border-radius: 18px;
			min-width: 0;
			min-height: 0;
			resize: none;
		}
		.dialog-head,
		.dialog-controls,
		.dialog-foot {
			padding-left: 18px;
			padding-right: 18px;
		}
		.dialog-section-head,
		.dialog-foot {
			flex-direction: column;
			align-items: stretch;
		}
		.dialog-foot-buttons {
			justify-content: stretch;
		}
		.dialog-foot-buttons :global(button) {
			flex: 1;
		}
	}
</style>

<script lang="ts">
	import { autoFixFinding, applyAutoFix, type AutoFixResult, type LineEdit } from '$lib/services/docReviewService';
	import type { FindingItem } from '$lib/services/docReviewService';

	let {
		finding,
		dark = true,
		onsaved,
		oncancel
	}: {
		finding: FindingItem;
		dark?: boolean;
		onsaved: () => void;
		oncancel: () => void;
	} = $props();

	type Phase = 'loading' | 'result' | 'saving';

	let phase = $state<Phase>('loading');
	let result = $state<AutoFixResult | null>(null);
	let errorMsg = $state('');

	async function runFix() {
		phase = 'loading';
		result = null;
		errorMsg = '';
		try {
			result = await autoFixFinding(finding.id);
		} catch (e) {
			errorMsg = e instanceof Error ? e.message : 'Auto-fix failed';
			phase = 'result';
			return;
		}
		phase = 'result';
	}

	async function save() {
		if (!result?.fixable || !result.corrected?.length) return;
		phase = 'saving';
		try {
			await applyAutoFix(finding.id, result.corrected, result.model_name ?? '');
			onsaved();
		} catch (e) {
			errorMsg = e instanceof Error ? e.message : 'Failed to apply fix';
			phase = 'result';
		}
	}

	function onBackdrop(e: MouseEvent) {
		if (e.target === e.currentTarget && phase !== 'saving') oncancel();
	}

	// Start the LLM call immediately on mount.
	runFix();
</script>

<div class="backdrop" class:dark role="presentation" onclick={onBackdrop}>
	<div class="dialog" role="dialog" aria-modal="true" aria-label="LLM Auto Fix">
		<h2 class="title">LLM Auto Fix</h2>

		{#if phase === 'loading'}
			<div class="loading-state">
				<div class="spinner"></div>
				<span class="loading-text">Running model on finding…</span>
			</div>
		{:else}
			{#if errorMsg}
				<div class="error-banner">{errorMsg}</div>
			{/if}

			{#if result}
				{#if result.model_name}
					<div class="meta-row">
						<span class="meta-label">Model</span>
						<span class="meta-value">{result.model_name}</span>
						{#if result.elapsed_ms}
							<span class="meta-sep">·</span>
							<span class="meta-value">{result.elapsed_ms.toLocaleString()} ms</span>
						{/if}
					</div>
				{/if}

				<div class="section-label">Reason</div>
				<div class="reason-box">
					{#if finding.description}{finding.description}{/if}
					{#if finding.suggestion}
						{#if finding.description}<br />{/if}
						<em>Suggestion:</em> {finding.suggestion}
					{/if}
				</div>

				{#if result.fixable && result.original?.length}
					<div class="section-label">Offending lines → proposed fix</div>
					<div class="lines-grid">
						{#each result.original as orig, i}
							{@const fix = result.corrected?.[i]}
							<div class="line-row">
								<span class="line-no">L{orig.line_no}</span>
								<div class="line-pair">
									<div class="line-before">{orig.content}</div>
									{#if fix && fix.content !== orig.content}
										<div class="line-after">{fix.content}</div>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				{:else if !result.fixable}
					<div class="unfixable-msg">{result.message || 'This finding could not be auto-fixed.'}</div>
				{/if}
			{/if}
		{/if}

		<div class="actions">
			<button class="btn ghost" onclick={oncancel} disabled={phase === 'saving'}>Cancel</button>
			<button
				class="btn"
				onclick={runFix}
				disabled={phase === 'loading' || phase === 'saving'}
			>Retry</button>
			<button
				class="btn primary"
				onclick={save}
				disabled={phase !== 'result' || !result?.fixable || !result.corrected?.length}
			>{phase === 'saving' ? 'Saving…' : 'Save'}</button>
		</div>
	</div>
</div>

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1000;
		padding: 1rem;
	}
	.dialog {
		width: min(640px, 100%);
		max-height: 90vh;
		overflow-y: auto;
		background: #ffffff;
		color: #111827;
		border-radius: 12px;
		padding: 1.25rem 1.5rem;
		box-shadow: 0 20px 60px rgba(0, 0, 0, 0.35);
		font-family: system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif;
	}
	.backdrop.dark .dialog {
		background: #1f2333;
		color: #e2e8f0;
		border: 1px solid #2d3348;
	}
	.title {
		margin: 0 0 1rem;
		font-size: 1.15rem;
	}

	/* Loading */
	.loading-state {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 1.5rem 0;
	}
	.spinner {
		width: 20px;
		height: 20px;
		border: 2px solid #d1d5db;
		border-top-color: #6366f1;
		border-radius: 50%;
		animation: spin 0.7s linear infinite;
		flex-shrink: 0;
	}
	.backdrop.dark .spinner {
		border-color: #2d3348;
		border-top-color: #818cf8;
	}
	@keyframes spin {
		to { transform: rotate(360deg); }
	}
	.loading-text {
		font-size: 0.9rem;
		color: #6b7280;
	}
	.backdrop.dark .loading-text {
		color: #94a3b8;
	}

	/* Meta row */
	.meta-row {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.78rem;
		margin-bottom: 0.85rem;
		color: #6b7280;
	}
	.backdrop.dark .meta-row {
		color: #94a3b8;
	}
	.meta-label {
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		font-size: 0.7rem;
	}
	.meta-sep { opacity: 0.5; }

	/* Sections */
	.section-label {
		font-size: 0.72rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: #6b7280;
		margin: 0.5rem 0 0.35rem;
	}
	.backdrop.dark .section-label { color: #64748b; }

	.reason-box {
		font-size: 0.86rem;
		line-height: 1.5;
		padding: 0.55rem 0.7rem;
		border-radius: 6px;
		background: rgba(99, 102, 241, 0.08);
		border: 1px solid rgba(99, 102, 241, 0.25);
		color: #374151;
		margin-bottom: 0.85rem;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.backdrop.dark .reason-box {
		background: rgba(129, 140, 248, 0.12);
		border-color: rgba(129, 140, 248, 0.3);
		color: #cbd5e1;
	}

	/* Lines diff */
	.lines-grid {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		margin-bottom: 1rem;
	}
	.line-row {
		display: flex;
		gap: 0.5rem;
		align-items: flex-start;
	}
	.line-no {
		flex: 0 0 auto;
		font-size: 0.72rem;
		color: #6b7280;
		min-width: 2.75rem;
		font-variant-numeric: tabular-nums;
		padding-top: 0.45rem;
	}
	.backdrop.dark .line-no { color: #64748b; }
	.line-pair {
		flex: 1 1 auto;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}
	.line-before,
	.line-after {
		font-size: 0.84rem;
		line-height: 1.45;
		padding: 0.4rem 0.6rem;
		border-radius: 5px;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.line-before {
		background: rgba(220, 38, 38, 0.07);
		border: 1px solid rgba(220, 38, 38, 0.2);
		color: #374151;
		text-decoration: line-through;
		opacity: 0.75;
	}
	.backdrop.dark .line-before {
		background: rgba(248, 113, 113, 0.1);
		border-color: rgba(248, 113, 113, 0.25);
		color: #94a3b8;
	}
	.line-after {
		background: rgba(16, 185, 129, 0.08);
		border: 1px solid rgba(16, 185, 129, 0.3);
		color: #374151;
	}
	.backdrop.dark .line-after {
		background: rgba(16, 185, 129, 0.12);
		border-color: rgba(16, 185, 129, 0.35);
		color: #cbd5e1;
	}

	.unfixable-msg {
		font-size: 0.86rem;
		line-height: 1.5;
		color: #dc2626;
		padding: 0.5rem 0;
		margin-bottom: 0.5rem;
	}
	.backdrop.dark .unfixable-msg { color: #f87171; }

	.error-banner {
		font-size: 0.84rem;
		color: #dc2626;
		padding: 0.5rem 0.7rem;
		border-radius: 6px;
		background: rgba(220, 38, 38, 0.07);
		border: 1px solid rgba(220, 38, 38, 0.2);
		margin-bottom: 0.75rem;
	}
	.backdrop.dark .error-banner {
		background: rgba(248, 113, 113, 0.1);
		border-color: rgba(248, 113, 113, 0.25);
		color: #f87171;
	}

	/* Actions */
	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 1.25rem;
		border-top: 1px solid #e4e6eb;
		padding-top: 1rem;
	}
	.backdrop.dark .actions { border-top-color: #2d3348; }
	.btn {
		padding: 0.45rem 0.9rem;
		border: 1px solid #d1d5db;
		border-radius: 6px;
		background: #f3f4f6;
		color: #111827;
		font: inherit;
		font-size: 0.84rem;
		cursor: pointer;
		transition: background 0.15s, opacity 0.15s;
	}
	.backdrop.dark .btn {
		background: #2d3348;
		color: #e2e8f0;
		border-color: #3a4158;
	}
	.btn:hover:not(:disabled) { background: #e5e7eb; }
	.backdrop.dark .btn:hover:not(:disabled) { background: #3a4158; }
	.btn:disabled { opacity: 0.45; cursor: not-allowed; }
	.btn.primary {
		background: #6366f1;
		border-color: #6366f1;
		color: #ffffff;
	}
	.btn.primary:hover:not(:disabled) { background: #4f46e5; }
	.btn.ghost {
		background: transparent;
	}
</style>

<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import type { SummaryCategoryNode } from './summary-types';

	type DialogMode = 'rename' | 'metadata' | 'add' | 'delete' | 'merge' | 'split' | null;

	let {
		open = $bindable(false),
		mode = null,
		node = null,
		availableNodes = [],
		onConfirm = () => {},
		onCancel = () => {}
	}: {
		open?: boolean;
		mode?: DialogMode;
		node?: SummaryCategoryNode | null;
		availableNodes?: SummaryCategoryNode[];
		onConfirm?: (payload: Record<string, unknown>) => void;
		onCancel?: () => void;
	} = $props();

	let label = $state('');
	let desc = $state('');
	let categoryType = $state('');
	let confidence = $state('0.75');
	let keywords = $state('');
	let mergeTargetId = $state('');
	let splitLabels = $state('Branch A\nBranch B');

	$effect(() => {
		if (!node || !open) return;
		label = node.label;
		desc = node.metadata?.desc ?? '';
		categoryType = node.metadata?.category_type ?? 'topic';
		confidence = String(node.metadata?.confidence ?? 0.75);
		keywords = (node.metadata?.keywords ?? []).join(', ');
		mergeTargetId = availableNodes.find((candidate) => candidate.id !== node.id)?.id ?? '';
		splitLabels = `${node.label} A\n${node.label} B`;
	});

	function close() {
		open = false;
		onCancel();
	}

	function confirm() {
		onConfirm({
			label,
			desc,
			categoryType,
			confidence: Number(confidence),
			keywords: keywords
				.split(',')
				.map((item) => item.trim())
				.filter(Boolean),
			mergeTargetId,
			splitLabels: splitLabels
				.split(/\r?\n/)
				.map((item) => item.trim())
				.filter(Boolean)
		});
		open = false;
	}
</script>

{#if open && node && mode}
	<div
		class="overlay"
		role="presentation"
		tabindex="-1"
		onclick={close}
		onkeydown={(event) => {
			if (event.key === 'Escape') close();
		}}
	>
		<div
			class="dialog"
			role="dialog"
			aria-modal="true"
			tabindex="0"
			onclick={(event) => event.stopPropagation()}
			onkeydown={(event) => event.stopPropagation()}
		>
			<div class="dialog-head">
				<div>
					<div class="eyebrow">{m.summary_node_dialog_mocked_phase_1_action()}</div>
					<h2>
						{#if mode === 'rename'}{m.summary_node_dialog_rename_node()}{/if}
						{#if mode === 'metadata'}{m.summary_node_dialog_edit_metadata()}{/if}
						{#if mode === 'add'}{m.summary_node_dialog_add_child_node()}{/if}
						{#if mode === 'delete'}{m.summary_node_dialog_delete_node()}{/if}
						{#if mode === 'merge'}{m.summary_node_dialog_merge_nodes()}{/if}
						{#if mode === 'split'}{m.summary_node_dialog_split_node()}{/if}
					</h2>
					<p>{node.categoryPath}</p>
				</div>
				<button type="button" class="close-btn" onclick={close}>{m.summary_node_dialog_close()}</button>
			</div>

			{#if mode === 'delete'}
				<div class="danger-copy">
					{m.summary_node_dialog_this_removes_the_selected_category()}
				</div>
			{:else}
				<div class="form-grid">
					{#if mode === 'rename' || mode === 'add'}
						<label>
							<span>{m.summary_node_dialog_label()}</span>
							<input bind:value={label} placeholder={m.summary_node_dialog_category_label()} />
						</label>
					{/if}

					{#if mode === 'metadata'}
						<label class="wide">
							<span>{m.summary_node_dialog_description()}</span>
							<textarea bind:value={desc} rows="4"></textarea>
						</label>
						<label>
							<span>{m.summary_node_dialog_category_type()}</span>
							<input bind:value={categoryType} placeholder={m.summary_node_dialog_topic()} />
						</label>
						<label>
							<span>{m.summary_node_dialog_confidence()}</span>
							<input bind:value={confidence} type="number" min="0" max="1" step="0.01" />
						</label>
						<label class="wide">
							<span>{m.summary_node_dialog_keywords()}</span>
							<input bind:value={keywords} placeholder={m.summary_node_dialog_filing_evidence_review()} />
						</label>
					{/if}

					{#if mode === 'merge'}
						<label class="wide">
							<span>{m.summary_node_dialog_merge_into()}</span>
							<select bind:value={mergeTargetId}>
								{#each availableNodes.filter((candidate) => candidate.id !== node.id) as candidate}
									<option value={candidate.id}>{candidate.categoryPath}</option>
								{/each}
							</select>
						</label>
					{/if}

					{#if mode === 'split'}
						<label class="wide">
							<span>{m.summary_node_dialog_new_branch_labels()}</span>
							<textarea bind:value={splitLabels} rows="5"></textarea>
						</label>
					{/if}
				</div>
			{/if}

			<div class="actions">
				<button type="button" class="secondary" onclick={close}>{m.summary_node_dialog_cancel()}</button>
				<button type="button" class="primary" onclick={confirm}>
					{mode === 'delete' ? m.summary_node_dialog_delete_node() : m.summary_node_dialog_apply_mock_update()}
				</button>
			</div>
		</div>
	</div>
{/if}

<style>
	.overlay {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 1.5rem;
		background: rgba(2, 6, 23, 0.62);
		backdrop-filter: blur(10px);
		z-index: 20;
	}

	.dialog {
		width: min(720px, 100%);
		border-radius: 22px;
		border: 1px solid rgba(148, 163, 184, 0.16);
		background: #111827;
		padding: 1.2rem;
		box-shadow: 0 30px 80px rgba(15, 23, 42, 0.5);
	}

	.dialog-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.eyebrow {
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: #94a3b8;
	}

	h2 {
		margin: 0.3rem 0 0.2rem;
		font-size: 1.2rem;
	}

	p {
		margin: 0;
		font-size: 0.86rem;
		color: #94a3b8;
	}

	.close-btn,
	.secondary,
	.primary {
		border-radius: 12px;
		padding: 0.7rem 1rem;
		border: 1px solid rgba(148, 163, 184, 0.18);
		cursor: pointer;
	}

	.close-btn,
	.secondary {
		background: transparent;
		color: #cbd5e1;
	}

	.primary {
		background: linear-gradient(135deg, #6366f1, #8b5cf6);
		border-color: transparent;
		color: white;
		font-weight: 700;
	}

	.form-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.85rem;
	}

	label {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	label.wide {
		grid-column: 1 / -1;
	}

	span {
		font-size: 0.78rem;
		font-weight: 700;
		color: #94a3b8;
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	input,
	textarea,
	select {
		border-radius: 14px;
		border: 1px solid rgba(148, 163, 184, 0.16);
		background: rgba(15, 23, 42, 0.78);
		padding: 0.8rem 0.9rem;
		color: #e2e8f0;
	}

	.danger-copy {
		border-radius: 16px;
		padding: 1rem;
		background: rgba(239, 68, 68, 0.1);
		color: #fecaca;
	}

	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-top: 1rem;
	}

	@media (max-width: 760px) {
		.form-grid {
			grid-template-columns: minmax(0, 1fr);
		}
	}
</style>

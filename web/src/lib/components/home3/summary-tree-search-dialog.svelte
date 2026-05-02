<script lang="ts">
	import { knowledgeStoreState } from './knowledge-store-state.svelte';

	export type SummaryTreeSearchFields = {
		recordId: string;
		title: string;
		docNo: string;
		fileName: string;
		docType: string;
		parserName: string;
		operation: string;
		procStatus: string;
		createStart: string;
		createEnd: string;
		modifyStart: string;
		modifyEnd: string;
	};

	let {
		open = $bindable(false),
		fields = $bindable({
			recordId: '',
			title: '',
			docNo: '',
			fileName: '',
			docType: 'all',
			parserName: '',
			operation: '',
			procStatus: 'all',
			createStart: '',
			createEnd: '',
			modifyStart: '',
			modifyEnd: ''
		}),
		onSearch = () => {},
		onReset = () => {}
	}: {
		open?: boolean;
		fields?: SummaryTreeSearchFields;
		onSearch?: () => void;
		onReset?: () => void;
	} = $props();

	const docTypeOptions = ['all', 'pdf', 'doc', 'excel', 'ppt', 'text', 'markdown'];
	const procStatusOptions = ['all', 'success', 'fail'];
</script>

{#if open}
	<div
		class="overlay"
		role="presentation"
		tabindex="-1"
		onclick={() => (open = false)}
		onkeydown={(event) => {
			if (event.key === 'Escape') open = false;
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
					<div class="eyebrow">kb.inputs Summary Browser</div>
					<h2>Find a record</h2>
					<p>Search by record identity, parser state, and time windows, following the Document Details pattern.</p>
					<div class="scope-copy">
						Store scope:
						{#if knowledgeStoreState.activeStore}
							<strong>{knowledgeStoreState.activeStore.ks_name}</strong>
							<span>#{knowledgeStoreState.activeStore.id}</span>
						{:else}
							<strong>No active store</strong>
						{/if}
					</div>
				</div>
				<button type="button" class="ghost" onclick={() => (open = false)}>Close</button>
			</div>

			<div class="section">
				<h3>Identity</h3>
				<div class="grid">
					<label><span>Record ID</span><input bind:value={fields.recordId} placeholder="84" /></label>
					<label><span>Type</span><select bind:value={fields.docType}>{#each docTypeOptions as option}<option value={option}>{option}</option>{/each}</select></label>
					<label class="wide"><span>Title contains</span><input bind:value={fields.title} placeholder="Input title, standard title…" /></label>
					<label><span>Doc No contains</span><input bind:value={fields.docNo} placeholder="GB/T 123…" /></label>
					<label class="wide"><span>File name contains</span><input bind:value={fields.fileName} placeholder="report, spec, drawing…" /></label>
				</div>
			</div>

			<div class="section">
				<h3>Processing Status</h3>
				<div class="grid">
					<label><span>Parser name</span><input bind:value={fields.parserName} placeholder="mineru, docling…" /></label>
					<label><span>Operation</span><input bind:value={fields.operation} placeholder="extract_metadata" /></label>
					<label><span>Proc status</span><select bind:value={fields.procStatus}>{#each procStatusOptions as option}<option value={option}>{option}</option>{/each}</select></label>
				</div>
			</div>

			<div class="section">
				<h3>Time Windows</h3>
				<div class="grid">
					<label><span>Create from</span><input type="datetime-local" bind:value={fields.createStart} /></label>
					<label><span>Create to</span><input type="datetime-local" bind:value={fields.createEnd} /></label>
					<label><span>Modify from</span><input type="datetime-local" bind:value={fields.modifyStart} /></label>
					<label><span>Modify to</span><input type="datetime-local" bind:value={fields.modifyEnd} /></label>
				</div>
			</div>

			<div class="actions">
				<button type="button" class="ghost" onclick={onReset}>Reset</button>
				<button type="button" class="primary" onclick={() => { onSearch(); open = false; }}>Search</button>
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
		background: rgba(2, 6, 23, 0.6);
		backdrop-filter: blur(10px);
		z-index: 15;
	}

	.dialog {
		width: min(920px, 100%);
		max-height: min(88vh, 900px);
		overflow: auto;
		border-radius: 24px;
		border: 1px solid rgba(148, 163, 184, 0.16);
		background: #111827;
		padding: 1.2rem;
		color: #e2e8f0;
	}

	.dialog-head {
		display: flex;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.eyebrow,
	span {
		font-size: 0.72rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: #94a3b8;
	}

	h2,
	h3,
	p {
		margin: 0;
	}

	h2 {
		margin: 0.35rem 0;
	}

	p {
		color: #94a3b8;
	}

	.scope-copy {
		margin-top: 0.7rem;
		font-size: 0.8rem;
		color: #94a3b8;
	}

	.scope-copy strong {
		color: #fbbf24;
		margin-left: 0.3rem;
	}

	.scope-copy span {
		margin-left: 0.25rem;
		font-size: 0.78rem;
	}

	.section {
		margin-bottom: 1rem;
		border-radius: 18px;
		background: rgba(15, 23, 42, 0.6);
		padding: 1rem;
	}

	.section h3 {
		margin-bottom: 0.8rem;
		font-size: 0.98rem;
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.85rem;
	}

	label {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.wide {
		grid-column: 1 / -1;
	}

	input,
	select {
		border-radius: 12px;
		border: 1px solid rgba(148, 163, 184, 0.16);
		background: rgba(2, 6, 23, 0.5);
		padding: 0.8rem 0.85rem;
		color: #e2e8f0;
	}

	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.7rem;
	}

	.ghost,
	.primary {
		border-radius: 12px;
		padding: 0.75rem 1rem;
		border: 1px solid rgba(148, 163, 184, 0.18);
		cursor: pointer;
	}

	.ghost {
		background: transparent;
		color: #cbd5e1;
	}

	.primary {
		background: linear-gradient(135deg, #6366f1, #8b5cf6);
		border-color: transparent;
		color: white;
		font-weight: 700;
	}

	@media (max-width: 760px) {
		.grid {
			grid-template-columns: minmax(0, 1fr);
		}
	}
</style>

<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { onMount } from 'svelte';
	import { getModelsTOML, type LLMModelEntry } from './llm-models-client';
	import {
		clearEmbeddingRecords,
		compareEmbeddings,
		createEmbedding,
		deleteEmbeddingRecord,
		listEmbeddingRecords,
		regenerateEmbeddingRecord,
		searchEmbeddingSimilarity,
		updateEmbeddingRecord,
		type EmbeddingRecord
	} from './embedding-client';

	let { darkMode = true }: { darkMode?: boolean } = $props();
	let models = $state<LLMModelEntry[]>([]);
	let modelKey = $state('');
	let recordModelKey = $state('');
	let content = $state('');
	let saveToDatabase = $state(false);
	let topN = $state(10);
	let page = $state(1);
	let records = $state<EmbeddingRecord[]>([]);
	let total = $state(0);
	let selectedIds = $state<number[]>([]);
	let comparison = $state<Array<{ id_a: number; id_b: number; similarity: number }>>([]);
	let matches = $state<Array<EmbeddingRecord & { similarity: number }>>([]);
	let searched = $state(false);
	let vectorPreview = $state<number[] | null>(null);
	let editingID = $state<number | null>(null);
	let editContent = $state('');
	let loading = $state(false);
	let busy = $state(false);
	let error = $state('');
	let notice = $state('');
	let dark = $derived(darkMode);
	let model = $derived(models.find((entry) => entry.key === modelKey));
	let maxChars = $derived(model && model.max_chars > 0 ? model.max_chars : 6000);
	let remainingBytes = $derived(maxChars - Array.from(content).length);
	let dimensionModels = $derived(
		models.filter(
			(entry) => entry.model_type === 'embedding' && entry.dimension === (model?.dimension ?? 0)
		)
	);
	let pageCount = $derived(Math.max(1, Math.ceil(total / 20)));

	onMount(loadModels);

	async function loadModels() {
		try {
			const result = await getModelsTOML();
			models = result.models.filter((entry) => entry.model_type === 'embedding');
			if (!modelKey && models.length) modelKey = models[0].key;
			await loadRecords();
		} catch (err) {
			error = message(err);
		}
	}

	async function loadRecords() {
		if (!model) return;
		loading = true;
		try {
			const result = await listEmbeddingRecords(model.dimension, recordModelKey, page);
			records = result.records;
			total = result.total;
			selectedIds = selectedIds.filter((id) => records.some((record) => record.id === id));
		} catch (err) {
			error = message(err);
		} finally {
			loading = false;
		}
	}

	async function runEmbedding() {
		if (!model || !content.trim()) return;
		busy = true;
		error = '';
		notice = '';
		matches = [];
		vectorPreview = null;
		try {
			const result = await createEmbedding(model.key, content, saveToDatabase);
			vectorPreview = result.embedding.slice(0, 8);
			notice = saveToDatabase
				? `Embedding saved as record ${result.record_id}.`
				: 'Embedding generated. It was not saved.';
			if (saveToDatabase) await loadRecords();
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	async function runSimilarity() {
		if (!model || !content.trim()) return;
		busy = true;
		error = '';
		notice = '';
		comparison = [];
		searched = true;
		try {
			matches = (
				await searchEmbeddingSimilarity(model.key, content, Math.max(1, Number(topN) || 10))
			).matches;
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	function toggleRecord(record: EmbeddingRecord, checked: boolean) {
		if (checked) {
			if (selectedIds.length >= 5) {
				error = m.embedding_select_no_more_than_five();
				return;
			}
			selectedIds = [...selectedIds, record.id];
		} else selectedIds = selectedIds.filter((id) => id !== record.id);
		error = '';
	}

	async function compareSelected() {
		if (!model || selectedIds.length < 2) return;
		busy = true;
		error = '';
		notice = '';
		try {
			comparison = (await compareEmbeddings(model.key, selectedIds)).pairs;
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	async function saveEdit(record: EmbeddingRecord) {
		busy = true;
		error = '';
		try {
			await updateEmbeddingRecord(record.id, record.model_key, editContent);
			editingID = null;
			await loadRecords();
			notice = m.embedding_record_updated_and_re_embedded();
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	async function regenerate(record: EmbeddingRecord) {
		busy = true;
		error = '';
		try {
			await regenerateEmbeddingRecord(record.id, record.model_key);
			await loadRecords();
			notice = 'Embedding regenerated.';
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	async function remove(record: EmbeddingRecord) {
		if (!confirm(`Delete embedding record ${record.id}?`)) return;
		try {
			await deleteEmbeddingRecord(record.id, record.model_key);
			selectedIds = selectedIds.filter((id) => id !== record.id);
			await loadRecords();
		} catch (err) {
			error = message(err);
		}
	}

	async function clearTable() {
		if (
			!model ||
			!confirm(
				m.embedding_delete_all_records_for_from({ model_name: model.model_name, dimension: model.dimension })
			)
		)
			return;
		try {
			await clearEmbeddingRecords(model.dimension);
			selectedIds = [];
			comparison = [];
			await loadRecords();
			notice = m.embedding_records_cleared();
		} catch (err) {
			error = message(err);
		}
	}

	function changeModel() {
		recordModelKey = '';
		page = 1;
		selectedIds = [];
		comparison = [];
		matches = [];
		searched = false;
		void loadRecords();
	}
	function message(err: unknown) {
		return String((err as Error)?.message ?? err);
	}
</script>

<section class:dark class="embedding-admin">
	<header>
		<p class="eyebrow">{m.embedding_system_admin_llm()}</p>
		<h2>{m.embedding_embedding()}</h2>
		<p>{m.embedding_generate_embeddings_and_compare_records()}</p>
	</header>
	{#if error}<div class="message error">{error}</div>{/if}
	{#if notice}<div class="message success">{notice}</div>{/if}
	<section class="card">
		<h3>{m.embedding_test_embedding()}</h3>
		<div class="controls">
			<label
				>{m.embedding_embedding_model()}<select bind:value={modelKey} onchange={changeModel}
					>{#each models as entry}<option value={entry.key}
							>{entry.key} — {entry.model_name} ({entry.dimension})</option
						>{/each}</select
				></label
			>
			<span class="dimension"
				>{m.embedding_dimension_table({ dimension: model?.dimension ?? '—', value: model
					? `testbed.embedding_${model.dimension}`
					: '—' })}</span
			>
		</div>
		<label
			>{m.embedding_content()}<textarea bind:value={content} rows="5" placeholder={m.embedding_enter_text_to_embed()}
			></textarea></label
		>
		<div class="actions generate-actions">
			<label class="check"
				><input type="checkbox" bind:checked={saveToDatabase} /> {m.embedding_save_embedding_to_database()}</label
			><span class:over-limit={remainingBytes < 0} class="remaining-bytes">{m.embedding_remaining_bytes({ remainingBytes })}</span>
			><button onclick={runEmbedding} disabled={busy || !content.trim() || remainingBytes < 0}
				>{busy ? m.embedding_working() : 'Generate embedding'}</button
			>
		</div>
		{#if vectorPreview}<p class="muted">
				{m.embedding_vector_dimension_first_values({ dimension: model?.dimension ?? '', value: vectorPreview
					.map((n) => n.toFixed(5))
					.join(', ') })}
			</p>{/if}
		<div class="similarity-form">
			<h4>{m.embedding_similarity_search()}</h4>
			<label>{m.embedding_top_n()}<input type="number" min="1" max="100" bind:value={topN} /></label><button
				class="secondary"
				onclick={runSimilarity}
				disabled={busy || !content.trim()}>{m.embedding_search_selected_model_s_records()}</button
			>
		</div>
		{#if matches.length}<div class="results">
				<h4>{m.embedding_most_similar_records()}</h4>
				{#each matches as match}<article>
						<strong>#{match.id}</strong><span
							>{m.embedding_similarity({ value: (match.similarity * 100).toFixed(2) })}</span
						>
						<p>{match.content}</p>
					</article>{/each}
			</div>{:else if searched}<p class="muted">{m.embedding_no_matching_records_for_this()}</p>{/if}
	</section>

	<section class="card">
		<div class="list-heading">
			<div>
				<h3>{m.embedding_embedding_records()}</h3>
				<p class="muted">{m.embedding_records({ total, value: model ? `testbed.embedding_${model.dimension}` : '' })}</p>
			</div>
			<div class="actions">
				<label
					>{m.embedding_filter_by_model()}<select
						bind:value={recordModelKey}
						onchange={() => {
							page = 1;
							selectedIds = [];
							void loadRecords();
						}}
						><option value="">{m.embedding_all_models()}</option>{#each dimensionModels as entry}<option
								value={entry.key}>{entry.key} — {entry.model_name}</option
							>{/each}</select
					></label
				><button class="danger" onclick={clearTable} disabled={!model}>{m.embedding_clear_records()}</button>
			</div>
		</div>
		<div class="actions compare">
			<span>{m.embedding_5_selected({ selectedIdsCount: selectedIds.length })}</span><button
				class="secondary"
				onclick={compareSelected}
				disabled={busy || selectedIds.length < 2}>{m.embedding_compare_selected_pairwise()}</button
			>
		</div>
		{#if comparison.length}<div class="pairs">
				{#each comparison as pair}<div>
						#{pair.id_a} ↔ #{pair.id_b}: <strong>{(pair.similarity * 100).toFixed(2)}%</strong>
					</div>{/each}
			</div>{/if}
		{#if loading}<p>{m.embedding_loading_records()}</p>{:else if records.length === 0}<p class="muted">
				{m.embedding_no_embedding_records_found()}
			</p>{:else}
			<div class="table-wrap">
				<table>
					<thead
						><tr
							><th>{m.embedding_select()}</th><th>{m.embedding_id()}</th><th>{m.embedding_model()}</th><th>{m.embedding_content()}</th><th>{m.embedding_time_ms()}</th><th>{m.embedding_chars()}</th><th>{m.embedding_tokens()}</th><th>{m.embedding_updated()}</th><th
								>{m.embedding_actions()}</th
							></tr
						></thead
					><tbody>
						{#each records as record}
							<tr
								><td
									><input
										aria-label={m.embedding_select_record({ id: record.id })}
										type="checkbox"
										checked={selectedIds.includes(record.id)}
										disabled={(record.model_key !== modelKey && !selectedIds.includes(record.id)) ||
											(selectedIds.length >= 5 && !selectedIds.includes(record.id))}
										onchange={(event) => toggleRecord(record, event.currentTarget.checked)}
									/></td
								><td>{record.id}</td><td>{record.model_name}</td><td class="content-cell"
									>{#if editingID === record.id}<textarea bind:value={editContent} rows="3"
										></textarea><button onclick={() => saveEdit(record)} disabled={busy}
											>{m.embedding_save()}</button
										><button class="secondary" onclick={() => (editingID = null)}>{m.embedding_cancel()}</button
										>{:else}{record.content}{/if}</td
								><td>{record.time_ms}</td><td>{record.num_chars}</td><td>{record.num_tokens}</td><td>{new Date(record.updated_at).toLocaleString()}</td><td class="row-actions"
									><button
										class="secondary"
										onclick={() => {
											editingID = record.id;
											editContent = record.content;
										}}>{m.embedding_edit()}</button
									><button class="secondary" onclick={() => regenerate(record)} disabled={busy}
										>{m.embedding_regenerate()}</button
									><button class="danger" onclick={() => remove(record)}>{m.embedding_delete()}</button></td
								></tr
							>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
		<div class="pagination">
			<button
				class="secondary"
				onclick={() => {
					page = Math.max(1, page - 1);
					void loadRecords();
				}}
				disabled={page <= 1 || loading}>{m.embedding_previous()}</button
			><span>{m.embedding_page_of({ page, pageCount })}</span><button
				class="secondary"
				onclick={() => {
					page = Math.min(pageCount, page + 1);
					void loadRecords();
				}}
				disabled={page >= pageCount || loading}>{m.embedding_next()}</button
			>
		</div>
	</section>
</section>

<style>
	.embedding-admin {
		display: grid;
		gap: 1rem;
		color: #1f2937;
		padding: 1.25rem;
		user-select: text;
		-webkit-user-select: text;
	}
	.embedding-admin * {
		user-select: text;
		-webkit-user-select: text;
	}
	.embedding-admin.dark {
		color: #e5e7eb;
	}
	header h2 {
		margin: 0.15rem 0;
		font-size: 1.5rem;
	}
	header p,
	.muted {
		color: #7b8493;
		margin: 0.25rem 0;
	}
	.eyebrow {
		color: #7085df;
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}
	.card {
		background: color-mix(in srgb, currentColor 4%, transparent);
		border: 1px solid color-mix(in srgb, currentColor 14%, transparent);
		border-radius: 0.7rem;
		padding: 1rem;
	}
	.card h3 {
		margin: 0 0 0.8rem;
	}
	.controls,
	.actions,
	.list-heading,
	.pagination {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.7rem;
		flex-wrap: wrap;
	}
	label {
		display: grid;
		gap: 0.35rem;
		font-size: 0.88rem;
	}
	.controls label {
		min-width: min(100%, 25rem);
	}
	select,
	textarea,
	input[type='number'] {
		color: inherit;
		background: color-mix(in srgb, currentColor 5%, transparent);
		border: 1px solid color-mix(in srgb, currentColor 22%, transparent);
		border-radius: 0.35rem;
		padding: 0.55rem;
		font: inherit;
	}
	textarea {
		width: 100%;
		resize: vertical;
		box-sizing: border-box;
	}
	.card > label {
		margin-top: 0.8rem;
	}
	.dimension {
		color: #818b9b;
		font-size: 0.86rem;
	}
	button {
		border: 0;
		border-radius: 0.35rem;
		padding: 0.55rem 0.8rem;
		color: white;
		background: #536ce0;
		cursor: pointer;
	}
	button:disabled {
		opacity: 0.5;
		cursor: default;
	}
	button.secondary {
		color: inherit;
		background: color-mix(in srgb, currentColor 13%, transparent);
	}
	button.danger {
		background: #b84646;
	}
	.check {
		display: flex;
		align-items: center;
	}
	.generate-actions { margin-top: 0.75rem; }
	.remaining-bytes { margin-left: auto; color: #8791a1; font-size: 0.86rem; }
	.remaining-bytes.over-limit { color: #ef8585; }
	.generate-actions > button { margin-top: 0.45rem; }
	.similarity-form {
		display: flex;
		align-items: end;
		gap: 0.75rem;
		flex-wrap: wrap;
		border-top: 1px solid color-mix(in srgb, currentColor 14%, transparent);
		margin-top: 1rem;
		padding-top: 0.8rem;
	}
	.similarity-form h4 {
		margin: 0 auto 0 0;
	}
	.similarity-form input {
		width: 5rem;
	}
	.message {
		padding: 0.65rem 0.8rem;
		border-radius: 0.4rem;
	}
	.error {
		color: #ef8585;
		background: #b8464622;
	}
	.success {
		color: #59b98c;
		background: #29956a22;
	}
	.results article {
		border-top: 1px solid color-mix(in srgb, currentColor 13%, transparent);
		padding: 0.55rem 0;
	}
	.results article span {
		margin-left: 1rem;
		color: #56b889;
	}
	.results article p {
		white-space: pre-wrap;
		margin: 0.35rem 0;
	}
	.table-wrap {
		overflow-x: auto;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th,
	td {
		text-align: left;
		vertical-align: top;
		padding: 0.6rem 0.45rem;
		border-bottom: 1px solid color-mix(in srgb, currentColor 12%, transparent);
	}
	th {
		font-size: 0.82rem;
		color: #8791a1;
	}
	.content-cell {
		min-width: 16rem;
		max-width: 34rem;
		white-space: pre-wrap;
	}
	.row-actions {
		min-width: 16rem;
	}
	.row-actions button {
		margin: 0.1rem;
	}
	.compare {
		justify-content: flex-start;
		margin: 0.5rem 0;
	}
	.pairs {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem 1rem;
		margin: 0.5rem 0;
	}
	.pagination {
		justify-content: center;
		margin-top: 0.8rem;
	}
</style>

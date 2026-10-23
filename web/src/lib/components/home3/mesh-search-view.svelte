<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import {
		getMeshEntity,
		searchMesh,
		type MeshEntity,
		type MeshSearchOptions,
		type MeshSearchRow
	} from '$lib/services/meshService';

	let { darkMode = false }: { darkMode?: boolean } = $props();
	let query = $state('');
	let field = $state('all_terms');
	let match = $state('all');
	let words = $state('full');
	let sort = $state('label');
	let limit = $state('50');
	let submitted = $state<MeshSearchOptions | null>(null);
	let rows = $state<MeshSearchRow[]>([]);
	let offset = $state(0);
	let hasMore = $state(false);
	let busy = $state(false);
	let failed = $state(false);
	let selected = $state('');
	let entity = $state<MeshEntity | null>(null);
	let detailBusy = $state(false);
	let detailFailed = $state(false);
	let request = 0;
	let detailRequest = 0;
	const fields = $derived([
		{ id: 'all_terms', label: m.mesh_search_all_terms() },
		{ id: 'descriptor', label: m.mesh_search_descriptors() },
		{ id: 'qualifier', label: m.mesh_search_qualifiers() },
		{ id: 'scr', label: m.mesh_search_scr() },
		{ id: 'id', label: m.mesh_identifier() },
		{ id: 'scope_note', label: m.mesh_pred_scope_note() },
		{ id: 'annotation', label: m.mesh_search_annotation() },
		{ id: 'registry', label: m.mesh_search_registry() },
		{ id: 'pharmacological_action', label: m.mesh_pred_pharmacological_action() },
		{ id: 'heading_mapped_to', label: m.mesh_search_mapped() },
		{ id: 'free_text', label: m.mesh_search_free_text() }
	]);
	function kindLabel(kind: string) {
		if (kind === 'Qualifier') return m.mesh_kind_qualifier();
		if (kind.startsWith('SCR_')) return m.mesh_search_scr();
		if (kind === 'TopicalDescriptor') return m.mesh_kind_topical_descriptor();
		if (kind === 'GeographicalDescriptor') return m.mesh_kind_geographical_descriptor();
		if (kind === 'PublicationType') return m.mesh_kind_publication_type();
		if (kind === 'Concept') return m.mesh_kind_concept();
		if (kind === 'Term') return m.mesh_kind_term();
		return kind;
	}
	async function run(options: MeshSearchOptions, start: number) {
		const ticket = ++request;
		submitted = options;
		offset = start;
		busy = true;
		failed = false;
		rows = [];
		hasMore = false;
		selected = '';
		entity = null;
		detailBusy = false;
		detailFailed = false;
		++detailRequest;
		try {
			const result = await searchMesh(options, start);
			if (ticket !== request) return;
			rows = result.items;
			hasMore = result.has_more;
		} catch {
			if (ticket === request) failed = true;
		} finally {
			if (ticket === request) busy = false;
		}
	}
	function submit(event: SubmitEvent) {
		event.preventDefault();
		if (query.trim()) void run({ q: query.trim(), field, match, words, sort, limit }, 0);
	}
	async function openRecord(id: string) {
		const ticket = ++detailRequest;
		selected = id;
		entity = null;
		detailBusy = true;
		detailFailed = false;
		try {
			const result = await getMeshEntity(id);
			if (ticket === detailRequest) entity = result;
		} catch {
			if (ticket === detailRequest) detailFailed = true;
		} finally {
			if (ticket === detailRequest) detailBusy = false;
		}
	}
</script>

<div class="mesh-search h-full min-h-0 overflow-y-auto select-text" class:dark={darkMode}>
	<header>
		<div>
			<h1>{m.mesh_search_title()}</h1>
			<p>{m.mesh_search_description()}</p>
		</div>
		<a href="https://meshb.nlm.nih.gov/search" target="_blank" rel="noreferrer"
			>{m.mesh_search_nlm()} ↗</a
		>
	</header>
	<form onsubmit={submit}>
		<label for="mesh-query">{m.mesh_search_query()}</label>
		<div class="query-row">
			<input
				id="mesh-query"
				type="search"
				bind:value={query}
				maxlength="300"
				required
				placeholder={m.mesh_search_hint()}
			/>
			<button class="primary" type="submit" disabled={busy || !query.trim()}
				>{busy ? m.mesh_loading() : m.mesh_search_submit()}</button
			>
		</div>
		<div class="options">
			<label
				>{m.mesh_search_field()}
				<select bind:value={field}
					>{#each fields as option (option.id)}<option value={option.id}>{option.label}</option
						>{/each}</select
				>
			</label>
			<label
				>{m.mesh_search_matching()}
				<select bind:value={match}>
					<option value="exact">{m.mesh_search_exact()}</option>
					<option value="all">{m.mesh_search_all()}</option>
					<option value="any">{m.mesh_search_any()}</option>
				</select>
			</label>
			<label
				>{m.mesh_search_word_matching()}
				<select bind:value={words} disabled={match === 'exact'}>
					<option value="full">{m.mesh_search_full()}</option>
					<option value="substring">{m.mesh_search_substring()}</option>
				</select>
			</label>
			<label
				>{m.mesh_search_sort()}
				<select bind:value={sort}>
					<option value="label">{m.mesh_search_ascending()}</option>
					<option value="label_desc">{m.mesh_search_descending()}</option>
					<option value="id">{m.mesh_identifier()}</option>
				</select>
			</label>
			<label
				>{m.mesh_search_page_size()}
				<select bind:value={limit}
					>{#each [20, 50, 100] as size}<option value={String(size)}>{size}</option>{/each}</select
				>
			</label>
		</div>
		<p class="source-note">{m.mesh_search_source()}</p>
	</form>
	<div class="workspace">
		<section aria-label={m.mesh_search_results()} aria-busy={busy}>
			<h2>{m.mesh_search_results()}</h2>
			<div aria-live="polite">
				{#if busy}<p class="status">{m.mesh_loading()}</p>
				{:else if failed}<p class="status error" role="alert">{m.mesh_search_failed()}</p>
					<button type="button" onclick={() => submitted && run(submitted, offset)}
						>{m.mesh_search_retry()}</button
					>
				{:else if !submitted}<p class="status">{m.mesh_search_start()}</p>
				{:else if rows.length === 0}<p class="status">{m.mesh_no_results()}</p>
				{:else}<p class="range">
						{m.mesh_search_range({
							start: String(offset + 1),
							end: String(offset + rows.length),
							query: submitted.q
						})}
					</p>{/if}
			</div>
			<ul class="results">
				{#each rows as row (row.id)}
					<li>
						<button
							class:chosen={selected === row.id}
							type="button"
							onclick={() => openRecord(row.id)}
							aria-pressed={selected === row.id}
						>
							<strong>{row.label || row.id}</strong>
							<span><code>{row.id}</code> · {kindLabel(row.kind)}</span>
						</button>
					</li>
				{/each}
			</ul>
			{#if submitted && !failed}
				<nav class="pagination" aria-label={m.mesh_search_pagination()}>
					<button
						type="button"
						disabled={busy || offset === 0}
						onclick={() =>
							submitted && run(submitted, Math.max(0, offset - Number(submitted.limit)))}
						>{m.mesh_search_previous()}</button
					>
					<button
						type="button"
						disabled={busy || !hasMore}
						onclick={() => submitted && run(submitted, offset + Number(submitted.limit))}
						>{m.mesh_search_next()}</button
					>
				</nav>
			{/if}
		</section>
		<aside aria-label={m.mesh_detail_title()} aria-busy={detailBusy}>
			<h2>{m.mesh_detail_title()}</h2>
			{#if !selected}<p class="status">{m.mesh_detail_empty()}</p>
			{:else if detailBusy}<p class="status" role="status">{m.mesh_loading()}</p>
			{:else if detailFailed}<p class="status error" role="alert">
					{m.mesh_search_detail_failed()}
				</p>
				<button type="button" onclick={() => openRecord(selected)}>{m.mesh_search_retry()}</button>
			{:else if entity}
				<h3>{entity.label || entity.id}</h3>
				<p><code>{entity.id}</code> · {kindLabel(entity.kind)}</p>
				<a
					href={`https://meshb.nlm.nih.gov/record/ui?ui=${encodeURIComponent(entity.id)}`}
					target="_blank"
					rel="noreferrer">{m.mesh_search_official_record()} ↗</a
				>
				<dl>
					{#each entity.properties as property (property.predicate)}
						<dt>
							{property.predicate === 'scopeNote' ? m.mesh_pred_scope_note() : property.predicate}
						</dt>
						<dd>
							{#each property.values as value}
								{#if value.id}<button
										class="record-link"
										type="button"
										onclick={() => openRecord(value.id!)}>{value.label || value.id}</button
									>
								{:else}<p lang={value.lang || undefined}>{value.text}</p>{/if}
							{/each}
						</dd>
					{/each}
				</dl>
			{/if}
		</aside>
	</div>
</div>

<style>
	.mesh-search {
		--surface: #fff;
		--bg: #f4f6f8;
		--ink: #172b3a;
		--muted: #536878;
		--line: #d5dfe5;
		--accent: #17677a;
		background: var(--bg);
		color: var(--ink);
		padding: 1.5rem;
	}
	.mesh-search.dark {
		--surface: #202a36;
		--bg: #171e29;
		--ink: #e7eef4;
		--muted: #a7bac9;
		--line: #3a4a5a;
		--accent: #80d0dc;
	}
	header {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: 1rem;
		flex-wrap: wrap;
		margin-bottom: 1.5rem;
	}
	h1 {
		font-size: 1.6rem;
		font-weight: 650;
		letter-spacing: -0.025em;
	}
	h2 {
		font-size: 1rem;
		font-weight: 650;
		margin-bottom: 0.75rem;
	}
	h3 {
		font-size: 1.2rem;
		font-weight: 650;
		margin-bottom: 0.5rem;
	}
	header p,
	.source-note,
	.status,
	.range {
		color: var(--muted);
		font-size: 0.875rem;
	}
	header p {
		margin-top: 0.35rem;
	}
	a {
		color: var(--accent);
		text-decoration: underline;
		text-underline-offset: 3px;
		font-size: 0.875rem;
	}
	form {
		background: var(--surface);
		border: 1px solid var(--line);
		border-top: 3px solid var(--accent);
		padding: 1.25rem;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		font-size: 0.8rem;
		font-weight: 600;
	}
	.query-row {
		display: flex;
		gap: 0.65rem;
		margin-top: 0.4rem;
	}
	input,
	select,
	button {
		border: 1px solid var(--line);
		border-radius: 4px;
		padding: 0.6rem 0.75rem;
		background: var(--surface);
		color: var(--ink);
		font: inherit;
	}
	input {
		flex: 1;
		min-width: 0;
		font-size: 1rem;
	}
	select {
		min-width: 0;
		width: 100%;
		font-size: 0.85rem;
	}
	button {
		cursor: pointer;
		font-size: 0.875rem;
	}
	button:disabled {
		opacity: 0.5;
		cursor: default;
	}
	button.primary {
		background: #17677a;
		color: white;
		border-color: #17677a;
		padding-inline: 1.5rem;
	}
	input:focus-visible,
	select:focus-visible,
	button:focus-visible,
	a:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 3px;
	}
	.options {
		display: grid;
		grid-template-columns: 2fr 1fr 1fr 1.25fr 0.8fr;
		gap: 1rem;
		margin-top: 1.25rem;
	}
	.source-note {
		margin-top: 1rem;
		line-height: 1.5;
	}
	.workspace {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
		gap: 1.25rem;
		margin-top: 1.25rem;
	}
	section,
	aside {
		min-width: 0;
		border: 1px solid var(--line);
		padding: 1.25rem;
		background: var(--surface);
	}
	.status {
		padding-block: 1rem;
		line-height: 1.6;
	}
	.error {
		color: #cb5353;
	}
	.range {
		margin-bottom: 0.75rem;
	}
	.results {
		list-style: none;
		padding: 0;
		margin: 0;
	}
	.results button {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		width: 100%;
		text-align: left;
		border: 0;
		border-bottom: 1px solid var(--line);
		border-radius: 0;
		padding: 1rem 0.6rem;
	}
	.results button:hover,
	.results button.chosen {
		background: var(--bg);
		box-shadow: inset 3px 0 var(--accent);
	}
	.results strong {
		font-size: 0.95rem;
		font-weight: 600;
	}
	.results span {
		font-size: 0.75rem;
		color: var(--muted);
	}
	.pagination {
		display: flex;
		justify-content: space-between;
		gap: 0.75rem;
		margin-top: 1rem;
	}
	code {
		font-size: 0.8rem;
	}
	aside {
		overflow-wrap: anywhere;
	}
	aside > p {
		color: var(--muted);
		margin-bottom: 0.7rem;
		font-size: 0.85rem;
	}
	dt {
		font-size: 0.8rem;
		font-weight: 650;
		color: var(--muted);
		margin-top: 1.25rem;
	}
	dd {
		font-size: 0.875rem;
		line-height: 1.65;
		margin-top: 0.35rem;
		white-space: pre-wrap;
	}
	.record-link {
		color: var(--accent);
		padding: 0.2rem 0.4rem;
		margin: 0.2rem 0.35rem 0.2rem 0;
	}
	@media (max-width: 1100px) {
		.options {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
		.workspace {
			grid-template-columns: 1fr;
		}
	}
	@media (max-width: 600px) {
		.mesh-search {
			padding: 0.75rem;
		}
		.options {
			grid-template-columns: 1fr;
		}
		.query-row {
			flex-direction: column;
		}
	}
</style>

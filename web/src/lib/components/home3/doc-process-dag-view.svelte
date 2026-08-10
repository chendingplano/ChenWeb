<script lang="ts">
	import { onMount } from 'svelte';
	import {
		listDags,
		getDag,
		createDag,
		updateDag,
		deleteDag,
		listProcessors,
		defaultRuleFor,
		type DocProcessDag,
		type DocProcessDagDetail,
		type ProcessorSpec,
		type RuleInput
	} from './doc-process-dag-client';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	type RuleDraft = {
		key: string;
		name: string;
		target_processor: string;
		effect: string;
		predicate: string; // JSON text, '' = no predicate
		depends: string[];
	};

	type Draft = {
		name: string;
		display_name: string;
		description: string;
		is_system_default: boolean;
		processors: string[];
		rules: RuleDraft[];
	};

	const emptyDraft = (): Draft => ({
		name: '',
		display_name: '',
		description: '',
		is_system_default: false,
		processors: [],
		rules: []
	});

	let dags = $state<DocProcessDag[]>([]);
	let processors = $state<ProcessorSpec[]>([]);
	let loading = $state(false);
	let submitting = $state(false);
	let error = $state<string | null>(null);
	let info = $state<string | null>(null);
	let search = $state('');
	let searchDebounce: ReturnType<typeof setTimeout> | undefined;

	let editorOpen = $state(false);
	let editing = $state<DocProcessDag | null>(null); // null = create mode
	let draft = $state<Draft>(emptyDraft());

	let detail = $state<DocProcessDagDetail | null>(null);
	let detailLoading = $state(false);

	const defaultDag = $derived(dags.find((d) => d.is_system_default) ?? null);
	const totalProcessors = $derived(dags.reduce((acc, d) => acc + d.processors.length, 0));

	const pageBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const btn = $derived('#0F766E');
	const inputBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const panelBg = $derived(darkMode ? '#151A29' : '#FDFDFD');

	onMount(() => {
		loadAll();
	});

	async function loadAll() {
		loading = true;
		error = null;
		try {
			const [dagList, specs] = await Promise.all([listDags(search.trim()), listProcessors()]);
			dags = dagList;
			processors = specs;
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			loading = false;
		}
	}

	function onSearchInput() {
		if (searchDebounce) clearTimeout(searchDebounce);
		searchDebounce = setTimeout(() => loadAll(), 300);
	}

	function fmtDate(raw?: string): string {
		return raw ? new Date(raw).toLocaleString() : '—';
	}

	function classLabel(spec: ProcessorSpec): string {
		return spec.class ?? '';
	}

	// --- Editor (create / modify) ---

	function openCreate() {
		editing = null;
		draft = emptyDraft();
		error = null;
		info = null;
		editorOpen = true;
	}

	async function openEdit(dag: DocProcessDag) {
		error = null;
		info = null;
		try {
			const detailData = await getDag(dag.name);
			editing = dag;
			draft = {
				name: detailData.name,
				display_name: detailData.display_name ?? '',
				description: detailData.description ?? '',
				is_system_default: detailData.is_system_default,
				processors: [...detailData.processors],
				rules: rulesForDraft(detailData)
			};
			editorOpen = true;
		} catch (err) {
			error = String((err as Error).message ?? err);
		}
	}

	function rulesForDraft(detailData: DocProcessDagDetail): RuleDraft[] {
		return detailData.processors.map((p, i) => {
			const existing = detailData.rules.find((r) => (r.target_processor ?? '') === p);
			return existing
				? {
						key: `row-${i}`,
						name: existing.name || `gate-${p}`,
						target_processor: p,
						effect: existing.effect ?? 'require',
						predicate: existing.predicate ? JSON.stringify(existing.predicate) : '',
						depends: [...(existing.depends_on_processors ?? [])]
					}
				: {
						key: `row-${i}`,
						name: `gate-${p}`,
						target_processor: p,
						effect: 'require',
						predicate: '',
						depends: []
					};
		});
	}

	function isProcessorSelected(name: string): boolean {
		return draft.processors.includes(name);
	}

	function toggleProcessor(name: string) {
		error = null;
		if (draft.processors.includes(name)) {
			draft.processors = draft.processors.filter((p) => p !== name);
			draft.rules = draft.rules.filter((r) => r.target_processor !== name);
			// Drop edges that referenced the removed processor.
			for (const rule of draft.rules) {
				rule.depends = rule.depends.filter((d) => d !== name);
			}
		} else {
			draft.processors = [...draft.processors, name];
			draft.rules = [
				...draft.rules,
				{
					key: `row-${draft.rules.length}`,
					name: defaultRuleFor(name).name,
					target_processor: name,
					effect: 'require',
					predicate: '',
					depends: []
				}
			];
		}
	}

	function processorLabel(name: string): string {
		const spec = processors.find((p) => p.name === name);
		return spec ? `${name}${spec.phase ? ` · ${spec.phase}` : ''}` : name;
	}

	function closeEditor() {
		editorOpen = false;
		editing = null;
		error = null;
	}

	function buildRules(): RuleInput[] {
		const rules: RuleInput[] = [];
		for (const rule of draft.rules) {
			const input: RuleInput = {
				name: rule.name.trim() || `gate-${rule.target_processor}`,
				target_processor: rule.target_processor,
				effect: rule.effect,
				depends_on_processors: rule.depends
			};
			const predicateText = rule.predicate.trim();
			if (predicateText) {
				try {
					input.predicate = JSON.parse(predicateText) as Record<string, unknown>;
				} catch {
					throw new Error(`Invalid predicate JSON for gate "${input.name}": ${predicateText}`);
				}
			}
			rules.push(input);
		}
		return rules;
	}

	async function submitEditor() {
		error = null;
		info = null;
		if (!editing && !draft.name.trim()) {
			error = 'Name is required.';
			return;
		}
		if (draft.processors.length === 0) {
			error = 'A Doc Process DAG must have at least one doc processor.';
			return;
		}
		let rules: RuleInput[];
		try {
			rules = buildRules();
		} catch (err) {
			error = String((err as Error).message ?? err);
			return;
		}
		submitting = true;
		try {
			if (editing) {
				await updateDag(editing.name, {
					display_name: draft.display_name.trim() ? draft.display_name.trim() : null,
					description: draft.description.trim() ? draft.description.trim() : null,
					processors: draft.processors,
					is_system_default: draft.is_system_default,
					rules
				});
				info = 'Doc Process DAG updated.';
			} else {
				await createDag({
					name: draft.name.trim(),
					display_name: draft.display_name.trim() ? draft.display_name.trim() : undefined,
					description: draft.description.trim() ? draft.description.trim() : undefined,
					processors: draft.processors,
					is_system_default: draft.is_system_default,
					rules
				});
				info = 'Doc Process DAG created.';
			}
			editorOpen = false;
			editing = null;
			await loadAll();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			submitting = false;
		}
	}

	async function removeDag(dag: DocProcessDag) {
		if (
			!confirm(
				dag.is_system_default
					? 'This DAG is the system default. The system will refuse to delete it — promote another DAG first. Continue?'
					: `Delete Doc Process DAG "${dag.name}"? Every version, rule, and binding is removed permanently.`
			)
		) {
			return;
		}
		error = null;
		try {
			await deleteDag(dag.name);
			if (detail?.name === dag.name) detail = null;
			info = `Deleted Doc Process DAG "${dag.name}".`;
			await loadAll();
		} catch (err) {
			error = String((err as Error).message ?? err);
		}
	}

	async function viewDetail(dag: DocProcessDag) {
		if (detail?.name === dag.name) {
			detail = null;
			return;
		}
		detailLoading = true;
		error = null;
		try {
			detail = await getDag(dag.name);
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			detailLoading = false;
		}
	}
</script>

<div
	class="wrap"
	style:--page={pageBg}
	style:--card={card}
	style:--border={border}
	style:--heading={heading}
	style:--sub={sub}
	style:--btn={btn}
	style:--input-bg={inputBg}
	style:--panel-bg={panelBg}
>
	<header class="toolbar">
		<div>
			<h2>Doc Process DAG</h2>
			<p class="muted">
				A Doc Process DAG is a named doc process pipeline: a processor set, per-processor gates and
				depends_on_processors edges, and knowledge-store bindings. Names are unique; every DAG has
				at least one processor; one DAG is always the system default.
			</p>
		</div>
		<div class="toolbar-actions">
			<button class="ghost" onclick={loadAll} disabled={loading}>
				{loading ? 'Refreshing…' : 'Refresh'}
			</button>
			<button class="primary" onclick={openCreate}>{editorOpen ? 'Cancel' : '+ New DAG'}</button>
		</div>
	</header>

	{#if error}<div class="error">{error}</div>{/if}
	{#if info}<div class="info">{info}</div>{/if}

	<div class="summary-grid">
		<div class="summary-card">
			<div class="summary-label">Doc Process DAGs</div>
			<div class="summary-value">{dags.length}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">System Default</div>
			<div class="summary-value small">{defaultDag ? defaultDag.name : '— none —'}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">Processors in Use</div>
			<div class="summary-value">{totalProcessors}</div>
		</div>
	</div>

	<label class="search-row">
		<span class="search-label">Search</span>
		<input
			bind:value={search}
			oninput={onSearchInput}
			placeholder="Search by name or display name…"
		/>
	</label>

	{#if editorOpen}
		<form
			class="editor"
			onsubmit={(e) => {
				e.preventDefault();
				submitEditor();
			}}
		>
			<div class="editor-head">
				<h3>{editing ? `Modify: ${editing.name}` : 'New Doc Process DAG'}</h3>
				<button type="button" class="ghost compact-btn" onclick={closeEditor}>Close</button>
			</div>
			<div class="row two">
				<label>
					<span>Name</span>
					{#if editing}
						<input value={editing.name} disabled />
					{:else}
						<input bind:value={draft.name} required placeholder="e.g. financial-invoice-pipeline" />
					{/if}
				</label>
				<label>
					<span>Display name</span>
					<input bind:value={draft.display_name} placeholder="Financial Invoice Pipeline" />
				</label>
			</div>
			<label>
				<span>Description</span>
				<input bind:value={draft.description} placeholder="What this DAG is for…" />
			</label>

			<div class="toggle-row">
				<div>
					<span>System default</span>
					{#if editing && editing.is_system_default}
						<p class="muted inline-note">
							This is currently the system default. To change it, mark another DAG as default — the
							system keeps exactly one.
						</p>
					{:else if !editing && !defaultDag}
						<p class="muted inline-note">
							No DAG is the system default yet — this one will become the default automatically.
						</p>
					{/if}
				</div>
				<input
					type="checkbox"
					checked={draft.is_system_default}
					disabled={editing && editing.is_system_default}
					onchange={() => (draft.is_system_default = !draft.is_system_default)}
				/>
			</div>

			<div class="section">
				<div class="section-head">
					<span class="section-title">Processors</span>
					<span class="muted">At least one is required.</span>
				</div>
				{#if processors.length === 0}
					<p class="empty">No registered processors available.</p>
				{:else}
					<div class="processor-grid">
						{#each processors as spec (spec.name)}
							<label class="processor-chip" class:selected={isProcessorSelected(spec.name)}>
								<input
									type="checkbox"
									checked={isProcessorSelected(spec.name)}
									onchange={() => toggleProcessor(spec.name)}
								/>
								<span class="chip-name">{spec.name}</span>
								<span class="chip-meta"
									>{spec.phase ?? ''}{classLabel(spec) ? ` · ${classLabel(spec)}` : ''}</span
								>
							</label>
						{/each}
					</div>
				{/if}
			</div>

			{#if draft.processors.length > 0}
				<div class="section">
					<div class="section-head">
						<span class="section-title">Gates &amp; DAG edges</span>
						<span class="muted">
							depends_on_processors means the target runs only after those processors.
						</span>
					</div>
					<div class="rules-list">
						{#each draft.rules as rule (rule.key)}
							<div class="rule-card">
								<div class="rule-head">
									<div class="rule-target">
										<span>Gate for</span>
										<span class="rule-target-name">{processorLabel(rule.target_processor)}</span>
									</div>
									<label class="rule-effect">
										<span>Effect</span>
										<select bind:value={rule.effect}>
											<option value="require">require</option>
											<option value="enable">enable</option>
											<option value="skip">skip</option>
										</select>
									</label>
								</div>
								<label>
									<span>Rule name</span>
									<input bind:value={rule.name} placeholder={`gate-${rule.target_processor}`} />
								</label>
								{#if draft.processors.length > 1}
									<div class="depends-row">
										<span class="depends-label">Depends on</span>
										<div class="depends-chips">
											{#each draft.processors.filter((p) => p !== rule.target_processor) as dep (dep)}
												<label class="depends-chip">
													<input
														type="checkbox"
														checked={rule.depends.includes(dep)}
														onchange={() => {
															if (rule.depends.includes(dep)) {
																rule.depends = rule.depends.filter((d) => d !== dep);
															} else {
																rule.depends = [...rule.depends, dep];
															}
														}}
													/>
													<span>{dep}</span>
												</label>
											{/each}
										</div>
									</div>
								{/if}
								<label>
									<span
										>Predicate (optional JSON, e.g.
										&#123;"kind":"fact","path":"document.doc_kind","op":"eq","value":"invoice"&#125;)</span
									>
									<input
										bind:value={rule.predicate}
										placeholder="Leave empty for an unconditional gate."
									/>
								</label>
							</div>
						{/each}
					</div>
				</div>
			{/if}

			<div class="form-foot">
				<button class="primary" type="submit" disabled={submitting}>
					{submitting
						? 'Saving…'
						: editing
							? 'Save (new version if processors/rules changed)'
							: 'Create DAG'}
				</button>
			</div>
		</form>
	{/if}

	<h3>Doc Process DAGs</h3>
	{#if loading && dags.length === 0}
		<p class="empty">Loading…</p>
	{:else if dags.length === 0}
		<p class="empty">No Doc Process DAGs yet. Create one above.</p>
	{:else}
		<div class="dag-grid">
			{#each dags as dag (dag.name)}
				<div class="dag-card">
					<div class="dag-head">
						<div>
							<div class="dag-name">
								{dag.display_name || dag.name}
								{#if dag.is_system_default}
									<span class="default-badge">default</span>
								{/if}
							</div>
							<div class="cell-secondary">
								{dag.name} · v{dag.version}
								{#if dag.status !== 'active'}
									<span> · {dag.status}</span>
								{/if}
							</div>
						</div>
					</div>
					{#if dag.description}<p class="dag-desc">{dag.description}</p>{/if}
					<div class="badge-row">
						<span class="badge"
							>{dag.processors.length} processor{dag.processors.length === 1 ? '' : 's'}</span
						>
						<span class="badge">{dag.rule_count} gate{dag.rule_count === 1 ? '' : 's'}</span>
						<span class="badge">updated {fmtDate(dag.modify_time)}</span>
					</div>
					<div class="dag-processors">
						{#each dag.processors as p (p)}<span class="proc-chip">{p}</span>{/each}
					</div>
					<div class="card-foot">
						<button class="ghost compact-btn" onclick={() => viewDetail(dag)}>
							{detail?.name === dag.name ? 'Close' : 'View'}
						</button>
						<button class="ghost compact-btn" onclick={() => openEdit(dag)}>Modify</button>
						<button class="ghost compact-btn danger" onclick={() => removeDag(dag)}>Delete</button>
					</div>
				</div>
			{/each}
		</div>
	{/if}

	{#if detailLoading}
		<p class="empty">Loading detail…</p>
	{:else if detail}
		<div class="panel">
			<div class="panel-head">
				<h3>{detail.display_name || detail.name} — detail</h3>
				<button class="ghost compact-btn" onclick={() => (detail = null)}>Close</button>
			</div>
			<div class="detail-meta">
				<span><b>Name:</b> {detail.name}</span>
				<span><b>Version:</b> v{detail.version} ({detail.status})</span>
				<span><b>System default:</b> {detail.is_system_default ? 'yes' : 'no'}</span>
			</div>
			<div class="section">
				<div class="section-title">Processors</div>
				<div class="dag-processors">
					{#each detail.processors as p (p)}<span class="proc-chip">{p}</span>{/each}
				</div>
			</div>
			<div class="section">
				<div class="section-title">Gates &amp; DAG edges</div>
				{#if detail.rules.length === 0}
					<p class="empty">No gates defined.</p>
				{:else}
					<div class="table-wrap">
						<table>
							<thead>
								<tr>
									<th>Gate</th>
									<th>Target</th>
									<th>Effect</th>
									<th>Depends on</th>
								</tr>
							</thead>
							<tbody>
								{#each detail.rules as rule (rule.id)}
									<tr>
										<td>
											{rule.name}
											{#if rule.predicate}
												<div class="predicate" title={JSON.stringify(rule.predicate)}>
													{JSON.stringify(rule.predicate)}
												</div>
											{/if}
										</td>
										<td>{rule.target_processor || '—'}</td>
										<td>{rule.effect || '—'}</td>
										<td>
											{#if (rule.depends_on_processors ?? []).length === 0}
												—
											{:else}
												{#each rule.depends_on_processors ?? [] as dep (dep)}<span
														class="proc-chip mini">{dep}</span
													>{/each}
											{/if}
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			</div>
			<div class="section">
				<div class="section-title">Knowledge-store bindings</div>
				{#if detail.bindings.length === 0}
					<p class="empty">No bindings reference this DAG.</p>
				{:else}
					<div class="table-wrap">
						<table>
							<thead>
								<tr>
									<th>Name</th>
									<th>Kind</th>
									<th>Store</th>
									<th>Active</th>
									<th>Created</th>
								</tr>
							</thead>
							<tbody>
								{#each detail.bindings as binding (binding.id)}
									<tr>
										<td>{binding.name || '—'}</td>
										<td>{binding.binding_kind}</td>
										<td>{binding.ks_store_id ?? '—'}</td>
										<td>{binding.active ? 'yes' : 'no'}</td>
										<td>{fmtDate(binding.create_time)}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	.wrap {
		display: flex;
		flex-direction: column;
		gap: 16px;
		background: var(--page);
		min-height: 100%;
		padding: 16px 20px 32px;
	}
	.toolbar,
	.panel-head,
	.editor-head,
	.form-foot,
	.toolbar-actions,
	.toggle-row,
	.rule-head,
	.dag-head,
	.card-foot {
		display: flex;
	}
	.toolbar,
	.panel-head,
	.editor-head,
	.dag-head {
		justify-content: space-between;
		align-items: flex-start;
		gap: 12px;
	}
	.toolbar-actions {
		gap: 10px;
		flex-wrap: wrap;
	}
	.summary-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
		gap: 10px;
	}
	h2,
	h3 {
		margin: 0;
		color: var(--heading);
	}
	h2 {
		font-size: 20px;
	}
	h3 {
		font-size: 16px;
		margin-bottom: 4px;
	}
	.muted {
		color: var(--sub);
		font-size: 12px;
		margin: 4px 0 0;
		max-width: 720px;
	}
	.primary,
	.ghost {
		border-radius: 8px;
		padding: 8px 14px;
		font-size: 13px;
		cursor: pointer;
	}
	.primary {
		background: var(--btn);
		color: white;
		border: none;
	}
	.ghost {
		background: transparent;
		color: var(--heading);
		border: 1px solid var(--border);
	}
	.ghost.danger {
		color: #f87171;
		border-color: rgba(248, 113, 113, 0.4);
	}
	.compact-btn {
		padding: 6px 10px;
		font-size: 12px;
	}
	.primary:disabled,
	.ghost:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.summary-card,
	.panel,
	.editor,
	.dag-card {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 10px;
	}
	.summary-card {
		padding: 14px 16px;
	}
	.summary-label {
		font-size: 12px;
		color: var(--sub);
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
	.summary-value {
		margin-top: 6px;
		font-size: 22px;
		font-weight: 600;
		color: var(--heading);
	}
	.summary-value.small {
		font-size: 15px;
		word-break: break-word;
	}
	.error,
	.info {
		padding: 10px 12px;
		border-radius: 8px;
		font-size: 13px;
	}
	.error {
		background: rgba(248, 113, 113, 0.12);
		color: #f87171;
	}
	.info {
		background: rgba(15, 118, 110, 0.16);
		color: #5eead4;
	}
	.empty {
		color: var(--sub);
		font-style: italic;
		padding: 12px 8px;
	}
	.cell-secondary {
		font-size: 12px;
		color: var(--sub);
		margin-top: 2px;
	}

	.search-row {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.search-label {
		font-size: 12px;
		color: var(--sub);
	}
	input,
	select {
		background: var(--input-bg);
		color: var(--heading);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 10px;
		font-size: 13px;
		font-family: inherit;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 12px;
		color: var(--sub);
	}

	.editor {
		padding: 16px;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.row {
		display: grid;
		gap: 10px;
	}
	.row.two {
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
	}
	.toggle-row {
		align-items: center;
		justify-content: space-between;
		background: var(--panel-bg);
		border: 1px solid var(--border);
		padding: 10px 12px;
		border-radius: 8px;
		gap: 12px;
	}
	.toggle-row input[type='checkbox'] {
		width: 18px;
		height: 18px;
		flex-shrink: 0;
	}
	.toggle-row input[type='checkbox']:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
	.inline-note {
		margin: 2px 0 0;
		max-width: 480px;
	}
	.form-foot {
		justify-content: flex-end;
	}

	.section {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.section-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 10px;
	}
	.section-title {
		font-size: 13px;
		font-weight: 600;
		color: var(--heading);
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}

	.processor-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
		gap: 8px;
	}
	.processor-chip {
		flex-direction: row;
		align-items: center;
		gap: 8px;
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 10px;
		cursor: pointer;
		background: var(--input-bg);
	}
	.processor-chip.selected {
		border-color: var(--btn);
		background: rgba(15, 118, 110, 0.1);
	}
	.processor-chip input {
		width: 15px;
		height: 15px;
	}
	.chip-name {
		font-weight: 600;
		color: var(--heading);
	}
	.chip-meta {
		color: var(--sub);
		font-size: 11px;
	}

	.rules-list {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.rule-card {
		background: var(--panel-bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 12px;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.rule-head {
		align-items: flex-end;
		gap: 16px;
	}
	.rule-target {
		gap: 2px;
	}
	.rule-target-name {
		font-size: 13px;
		font-weight: 600;
		color: var(--heading);
	}
	.rule-effect {
		width: 160px;
	}
	.depends-row {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.depends-label {
		font-size: 12px;
		color: var(--sub);
	}
	.depends-chips {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}
	.depends-chip {
		flex-direction: row;
		align-items: center;
		gap: 5px;
		border: 1px solid var(--border);
		border-radius: 999px;
		padding: 4px 10px;
		font-size: 12px;
		background: var(--input-bg);
		cursor: pointer;
		color: var(--heading);
	}
	.depends-chip input {
		width: 14px;
		height: 14px;
	}

	.dag-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
		gap: 12px;
	}
	.dag-card {
		padding: 14px 16px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.dag-name {
		font-weight: 600;
		color: var(--heading);
		font-size: 15px;
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
	}
	.default-badge {
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		padding: 2px 7px;
		border-radius: 999px;
		background: rgba(15, 118, 110, 0.16);
		color: #5eead4;
		border: 1px solid rgba(15, 118, 110, 0.4);
	}
	.dag-desc {
		margin: 0;
		color: var(--sub);
		font-size: 12px;
	}
	.badge-row {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
	}
	.badge {
		font-size: 11px;
		padding: 3px 8px;
		border-radius: 999px;
		background: var(--panel-bg);
		border: 1px solid var(--border);
		color: var(--sub);
	}
	.dag-processors {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}
	.proc-chip {
		font-size: 11px;
		padding: 3px 9px;
		border-radius: 999px;
		background: rgba(15, 118, 110, 0.12);
		color: #5eead4;
		border: 1px solid rgba(15, 118, 110, 0.35);
	}
	.proc-chip.mini {
		padding: 1px 7px;
	}
	.card-foot {
		justify-content: flex-end;
		gap: 8px;
	}

	.panel {
		padding: 16px;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.detail-meta {
		display: flex;
		flex-wrap: wrap;
		gap: 18px;
		font-size: 13px;
		color: var(--heading);
	}
	.detail-meta b {
		color: var(--sub);
		font-weight: 500;
	}
	.predicate {
		margin-top: 3px;
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 11px;
		color: var(--sub);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		max-width: 260px;
	}
	.table-wrap {
		overflow-x: auto;
		margin-top: 4px;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th,
	td {
		padding: 9px 10px;
		border-top: 1px solid var(--border);
		font-size: 13px;
		color: var(--heading);
		text-align: left;
		vertical-align: top;
	}
	th {
		color: var(--sub);
		font-size: 12px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		border-top: none;
		padding-top: 0;
	}
</style>

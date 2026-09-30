<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { onMount } from 'svelte';

	import {
		applySync,
		createSyncItem,
		deleteSyncItem,
		getTableSchema,
		listSyncItems,
		listSyncableTables,
		previewSync,
		updateSyncItem,
		type ColumnInfo,
		type NaturalKeyCandidate,
		type SyncItem,
		type SyncItemDraft,
		type SyncItemKind,
		type TableRef
	} from './sync-data-client';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	let items = $state<SyncItem[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let info = $state<string | null>(null);
	let previewingID = $state<string | null>(null);
	let applyingID = $state<string | null>(null);
	let deletingID = $state<string | null>(null);
	let previewCounts = $state<Record<string, number>>({});

	// New Data Syncher: one form, shared between create (editingID === null)
	// and edit (editingID === the item being edited -- always origin=local,
	// since editing a learned or compiled item is never offered as a row
	// action).
	let showForm = $state(false);
	let editingID = $state<string | null>(null);
	let submitting = $state(false);
	// Filter is the only field left in Advanced (Columns/JSON Columns are
	// always auto-derived server-side now, see resolveItemShape) --
	// collapsed by default, expanded when editing an item that has one set.
	let showAdvanced = $state(false);

	// Every syncable table, for the Schema/Table pickers -- loaded once
	// alongside the item list and reused for both create and edit.
	let allTables = $state<TableRef[]>([]);
	let tablesLoading = $state(false);
	const schemaOptions = $derived([...new Set(allTables.map((t) => t.schema))].sort());
	function tablesForSchema(schema: string): string[] {
		return allTables.filter((t) => t.schema === schema).map((t) => t.table);
	}

	// The selected table's real shape -- backs the Cursor Column / File
	// Column / Natural Key pickers. Cleared whenever the selected table
	// changes (see onTableChange).
	let tableColumns = $state<ColumnInfo[]>([]);
	let naturalKeyCandidates = $state<NaturalKeyCandidate[]>([]);
	let tableInfoLoading = $state(false);
	let tableInfoError = $state<string | null>(null);

	type FormDraft = {
		id: string;
		kind: SyncItemKind;
		schema: string;
		table: string;
		cursor_col: string;
		// Comma-joined form of one selected natural_key_candidates entry --
		// a whole unique-constraint's column set is chosen as one atomic
		// option, since a natural key can span multiple columns.
		natural_key: string;
		filter: string;
		file_column: string;
		file_dir_env: string;
		file_dir_default_subdir: string;
	};

	function emptyFormDraft(): FormDraft {
		return {
			id: '',
			kind: 'table',
			schema: '',
			table: '',
			cursor_col: '',
			natural_key: '',
			filter: '',
			file_column: '',
			file_dir_env: '',
			file_dir_default_subdir: ''
		};
	}

	function splitTable(qualified: string): [string, string] {
		const idx = qualified.indexOf('.');
		return idx === -1 ? ['', qualified] : [qualified.slice(0, idx), qualified.slice(idx + 1)];
	}

	function toFormDraft(item: SyncItem): FormDraft {
		const [schema, table] = splitTable(item.table);
		return {
			id: item.item_id,
			kind: item.kind,
			schema,
			table,
			cursor_col: item.cursor_col,
			natural_key: item.natural_key.join(','),
			filter: item.filter ?? '',
			file_column: item.file_column ?? '',
			file_dir_env: item.file_dir_env ?? '',
			file_dir_default_subdir: item.file_dir_default_subdir ?? ''
		};
	}

	let formDraft = $state<FormDraft>(emptyFormDraft());

	onMount(() => {
		loadItems();
		loadTables();
	});

	async function loadTables() {
		tablesLoading = true;
		try {
			const response = await listSyncableTables();
			allTables = response.tables;
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			tablesLoading = false;
		}
	}

	// Fetches the real shape of schema.table -- does not touch
	// formDraft.cursor_col/natural_key/file_column, so it's safe to call
	// both when the user picks a new table (after the caller clears those)
	// and when pre-loading an existing item's pickers on edit (where the
	// caller wants the saved values to survive).
	async function fetchTableInfo(schema: string, table: string) {
		tableColumns = [];
		naturalKeyCandidates = [];
		tableInfoError = null;
		if (!schema || !table) return;
		tableInfoLoading = true;
		try {
			const response = await getTableSchema(schema, table);
			tableColumns = response.info.columns;
			naturalKeyCandidates = response.info.natural_key_candidates;
		} catch (err) {
			tableInfoError = String((err as Error).message ?? err);
		} finally {
			tableInfoLoading = false;
		}
	}

	function onSchemaChange() {
		formDraft.table = '';
		formDraft.cursor_col = '';
		formDraft.natural_key = '';
		formDraft.file_column = '';
		tableColumns = [];
		naturalKeyCandidates = [];
	}

	async function onTableChange() {
		formDraft.cursor_col = '';
		formDraft.natural_key = '';
		formDraft.file_column = '';
		await fetchTableInfo(formDraft.schema, formDraft.table);
		// Auto-select the natural key when it's unambiguous -- never the
		// primary key, since natural_key_candidates excludes it server-side.
		if (naturalKeyCandidates.length === 1) {
			formDraft.natural_key = naturalKeyCandidates[0].columns.join(',');
		}
	}

	async function loadItems() {
		loading = true;
		error = null;
		try {
			const response = await listSyncItems();
			items = response.items;
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			loading = false;
		}
	}

	async function runPreview(itemID: string) {
		previewingID = itemID;
		error = null;
		info = null;
		try {
			const result = await previewSync(itemID);
			previewCounts = { ...previewCounts, [itemID]: result.changed_row_count };
			info = m.sync_data_row_s_changed_since_the({ itemID, changed_row_count: result.changed_row_count });
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			previewingID = null;
		}
	}

	async function runApply(itemID: string) {
		applyingID = itemID;
		error = null;
		info = null;
		try {
			const result = await applySync(itemID);
			const { [itemID]: _discard, ...rest } = previewCounts;
			previewCounts = rest;
			info = m.sync_data_synced_row_s({ itemID, synced_row_count: result.synced_row_count });
			await loadItems();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			applyingID = null;
		}
	}

	function openCreateForm() {
		formDraft = emptyFormDraft();
		editingID = null;
		showForm = true;
		showAdvanced = false;
		tableColumns = [];
		naturalKeyCandidates = [];
		tableInfoError = null;
		error = null;
		info = null;
	}

	async function openEditForm(item: SyncItem) {
		formDraft = toFormDraft(item);
		editingID = item.item_id;
		showForm = true;
		showAdvanced = Boolean(formDraft.filter);
		error = null;
		info = null;
		// Populates the pickers' options without touching the values
		// toFormDraft() just set (fetchTableInfo never writes formDraft).
		await fetchTableInfo(formDraft.schema, formDraft.table);
	}

	function closeForm() {
		showForm = false;
		editingID = null;
	}

	function draftToPayload(): SyncItemDraft {
		return {
			id: formDraft.id.trim(),
			kind: formDraft.kind,
			table: `${formDraft.schema}.${formDraft.table}`,
			cursor_col: formDraft.cursor_col,
			natural_key: formDraft.natural_key ? formDraft.natural_key.split(',') : [],
			filter: formDraft.filter.trim(),
			file_column: formDraft.file_column,
			file_dir_env: formDraft.file_dir_env.trim(),
			file_dir_default_subdir: formDraft.file_dir_default_subdir.trim()
		};
	}

	async function submitForm() {
		error = null;
		info = null;
		submitting = true;
		try {
			const payload = draftToPayload();
			if (editingID) {
				await updateSyncItem(editingID, payload);
				info = m.sync_data_updated_its_sync_state_was({ editingID });
			} else {
				await createSyncItem(payload);
				info = `${payload.id}: created.`;
			}
			closeForm();
			await loadItems();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			submitting = false;
		}
	}

	async function runDelete(item: SyncItem) {
		const question = item.origin === 'learned' ? m.sync_data_delete_cached_sync_item_confirm : m.sync_data_delete_sync_item_confirm;
		if (!confirm(question({ item_id: item.item_id }))) {
			return;
		}
		deletingID = item.item_id;
		error = null;
		info = null;
		try {
			await deleteSyncItem(item.item_id);
			info = `${item.item_id}: deleted.`;
			await loadItems();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			deletingID = null;
		}
	}

	function fmtDate(raw?: string): string {
		if (!raw) return m.sync_data_never();
		return new Date(raw).toLocaleString();
	}

	const pageBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const btn = $derived(darkMode ? '#0F766E' : '#0F766E');
	const altBtn = $derived(darkMode ? '#818CF8' : '#6366F1');
	const inputBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const panelBg = $derived(darkMode ? '#151A29' : '#FDFDFD');
</script>

<div
	class="wrap"
	style:--page={pageBg}
	style:--card={card}
	style:--border={border}
	style:--heading={heading}
	style:--sub={sub}
	style:--btn={btn}
	style:--alt-btn={altBtn}
	style:--input-bg={inputBg}
	style:--panel-bg={panelBg}
>
	<header class="toolbar">
		<div>
			<h2>{m.sync_data_sync_data()}</h2>
			<p class="muted">
				{m.sync_data_pull_registered_reference_data_from()}
			</p>
		</div>
		<div class="toolbar-actions">
			<button class="ghost" onclick={loadItems} disabled={loading}>
				{loading ? m.sync_data_refreshing() : m.sync_data_refresh()}
			</button>
			<button
				class="primary"
				onclick={() => (showForm ? closeForm() : openCreateForm())}
			>
				{showForm ? m.sync_data_cancel() : m.sync_data_new_data_syncher()}
			</button>
		</div>
	</header>

	{#if error}
		<div class="error" role="alert">{error}</div>
	{:else if info}
		<div class="info" role="status">{info}</div>
	{/if}

	{#if showForm}
		<form
			class="create-form"
			onsubmit={(e) => {
				e.preventDefault();
				submitForm();
			}}
		>
			<h3>{editingID ? m.sync_data_edit({ editingID }) : m.sync_data_new_data_syncher_2()}</h3>
			<p class="form-intro">
				{m.sync_data_every_column_of_the_selected()}
			</p>
			<div class="row two">
				<label>
					<span>{m.sync_data_id()}</span>
					<input
						bind:value={formDraft.id}
						required
						disabled={editingID !== null}
						placeholder={m.sync_data_e_g_kb_videos()}
					/>
					<span class="hint">{m.sync_data_a_short_unique_name_for()}</span>
				</label>
				<label>
					<span>{m.sync_data_kind()}</span>
					<select bind:value={formDraft.kind}>
						<option value="table">{m.sync_data_table()}</option>
						<option value="table_with_files">{m.sync_data_table_with_files()}</option>
					</select>
					<span class="hint">{m.sync_data_table_with_files_also_copies()}</span>
				</label>
			</div>
			<div class="row two">
				<label>
					<span>{m.sync_data_schema()}</span>
					<select bind:value={formDraft.schema} onchange={onSchemaChange} required>
						<option value="" disabled>
							{tablesLoading ? m.sync_data_loading() : m.sync_data_select_a_schema()}
						</option>
						{#each schemaOptions as schema}
							<option value={schema}>{schema}</option>
						{/each}
					</select>
					<span class="hint">{m.sync_data_the_database_schema_the_table()}</span>
				</label>
				<label>
					<span>{m.sync_data_table_2()}</span>
					<select
						bind:value={formDraft.table}
						onchange={onTableChange}
						required
						disabled={!formDraft.schema}
					>
						<option value="" disabled>{m.sync_data_select_a_table()}</option>
						{#each tablesForSchema(formDraft.schema) as table}
							<option value={table}>{table}</option>
						{/each}
					</select>
					<span class="hint">{m.sync_data_schema_qualified_table_name_identical()}</span>
				</label>
			</div>
			{#if tableInfoLoading}
				<p class="hint">{m.sync_data_loading_table_columns()}</p>
			{:else if tableInfoError}
				<div class="error" role="alert">{tableInfoError}</div>
			{/if}
			<div class="row two">
				<label>
					<span>{m.sync_data_cursor_column()}</span>
					<select bind:value={formDraft.cursor_col} required disabled={tableColumns.length === 0}>
						<option value="" disabled>
							{tableColumns.length ? m.sync_data_select_a_column() : m.sync_data_select_a_table_first()}
						</option>
						{#each tableColumns as col}
							<option value={col.name}>{col.name} ({col.data_type})</option>
						{/each}
					</select>
					<span class="hint"
						>{m.sync_data_required_a_timestamp_column_bumped()}</span
					>
				</label>
				<label>
					<span>{m.sync_data_natural_key()}</span>
					<select
						bind:value={formDraft.natural_key}
						required
						disabled={naturalKeyCandidates.length === 0}
					>
						<option value="" disabled>
							{#if tableColumns.length === 0}
								{m.sync_data_select_a_table_first()}
							{:else if naturalKeyCandidates.length === 0}
								{m.sync_data_no_eligible_unique_constraint_on()}
							{:else}
								{m.sync_data_select()}
							{/if}
						</option>
						{#each naturalKeyCandidates as cand}
							<option value={cand.columns.join(',')}>{cand.columns.join(', ')}</option>
						{/each}
					</select>
					<span class="hint">
						{#if tableColumns.length > 0 && naturalKeyCandidates.length === 0}
							{m.sync_data_this_table_has_no_unique()}
						{:else}
							{m.sync_data_required_auto_selected_when_there()}
						{/if}
					</span>
				</label>
			</div>
			{#if formDraft.kind === 'table_with_files'}
				<div class="file-fields">
					<label>
						<span>{m.sync_data_file_column()}</span>
						<select
							bind:value={formDraft.file_column}
							required
							disabled={tableColumns.length === 0}
						>
							<option value="" disabled>
								{tableColumns.length ? m.sync_data_select_a_column() : m.sync_data_select_a_table_first()}
							</option>
							{#each tableColumns as col}
								<option value={col.name}>{col.name} ({col.data_type})</option>
							{/each}
						</select>
						<span class="hint">{m.sync_data_required_for_this_kind_the()}</span>
					</label>
					<div class="row two">
						<label>
							<span>{m.sync_data_file_dir_env_var()}</span>
							<input bind:value={formDraft.file_dir_env} placeholder={m.sync_data_e_g_video_dir()} />
							<span class="hint">{m.sync_data_where_this_instance_stores_synced()}</span>
						</label>
						<label>
							<span>{m.sync_data_file_dir_default_subdir()}</span>
							<input bind:value={formDraft.file_dir_default_subdir} placeholder={m.sync_data_e_g_videos()} />
							<span class="hint">{m.sync_data_fallback_subdirectory_if_the_env()}</span>
						</label>
					</div>
					<p class="hint">{m.sync_data_at_least_one_of_the()}</p>
				</div>
			{/if}

			<button
				type="button"
				class="advanced-toggle"
				onclick={() => (showAdvanced = !showAdvanced)}
			>
				{m.sync_data_advanced_optional_most_syncers_don({ value: showAdvanced ? '▾' : '▸' })}
			</button>
			{#if showAdvanced}
				<div class="file-fields">
					<label>
						<span>{m.sync_data_filter_raw_sql_where_fragment()}</span>
						<input bind:value={formDraft.filter} placeholder={m.sync_data_e_g_status_approved()} />
						<span class="hint"
							>{m.sync_data_this_is_the_one_field()}</span
						>
					</label>
				</div>
			{/if}

			<div class="form-foot">
				<button class="primary" type="submit" disabled={submitting}>
					{submitting ? m.sync_data_saving() : editingID ? m.sync_data_save_changes() : m.sync_data_create()}
				</button>
			</div>
		</form>
	{/if}

	<div class="panel">
		{#if loading}
			<div class="empty">{m.sync_data_loading_sync_items()}</div>
		{:else if items.length === 0}
			<div class="empty">{m.sync_data_no_sync_items_registered()}</div>
		{:else}
			<div class="table-wrap">
				<table>
					<thead>
						<tr>
							<th>{m.sync_data_item()}</th>
							<th>{m.sync_data_table_2()}</th>
							<th>{m.sync_data_kind()}</th>
							<th>{m.sync_data_origin()}</th>
							<th>{m.sync_data_last_synced()}</th>
							<th>{m.sync_data_last_row_count()}</th>
							<th>{m.sync_data_status()}</th>
							<th>{m.sync_data_action()}</th>
						</tr>
					</thead>
					<tbody>
						{#each items as item (item.item_id)}
							<tr>
								<td class="cell-primary">{item.item_id}</td>
								<td>{item.table}</td>
								<td class="cell-secondary">{item.kind}</td>
								<td class="cell-secondary">{item.origin}</td>
								<td>{fmtDate(item.last_synced_at)}</td>
								<td>{item.last_row_count}</td>
								<td>
									{#if item.last_error}
										<span class="status-error">{item.last_error}</span>
									{:else if item.item_id in previewCounts}
										<span class="cell-secondary"
											>{m.sync_data_row_s_pending({ previewCounts: previewCounts[item.item_id] })}</span
										>
									{:else}
										<span class="cell-secondary">{m.sync_data_ok()}</span>
									{/if}
								</td>
								<td>
									<div class="row-actions">
										<button
											class="ghost compact-btn"
											onclick={() => runPreview(item.item_id)}
											disabled={previewingID === item.item_id || applyingID === item.item_id}
										>
											{previewingID === item.item_id ? m.sync_data_previewing() : m.sync_data_preview()}
										</button>
										<button
											class="alt-btn compact-btn"
											onclick={() => runApply(item.item_id)}
											disabled={previewingID === item.item_id || applyingID === item.item_id}
										>
											{applyingID === item.item_id ? m.sync_data_syncing() : m.sync_data_sync()}
										</button>
										{#if item.origin === 'local'}
											<button class="ghost compact-btn" onclick={() => openEditForm(item)}>
												{m.sync_data_edit_2()}
											</button>
										{/if}
										{#if item.origin === 'local' || item.origin === 'learned'}
											<button
												class="ghost compact-btn"
												onclick={() => runDelete(item)}
												disabled={deletingID === item.item_id}
											>
												{deletingID === item.item_id ? m.sync_data_deleting() : m.sync_data_delete()}
											</button>
										{/if}
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
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
	.toolbar {
		display: flex;
		justify-content: space-between;
		align-items: flex-end;
		gap: 12px;
	}
	.toolbar-actions {
		display: flex;
		gap: 10px;
	}
	h2 {
		margin: 0;
		color: var(--heading);
		font-size: 20px;
	}
	h3 {
		margin: 0;
		color: var(--heading);
		font-size: 16px;
	}
	.muted {
		color: var(--sub);
		font-size: 12px;
		margin: 4px 0 0;
		max-width: 560px;
	}
	.primary,
	.ghost,
	.alt-btn {
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
	.alt-btn {
		background: var(--alt-btn);
		color: white;
		border: none;
	}
	.compact-btn {
		padding: 6px 10px;
		font-size: 12px;
	}
	.primary:disabled,
	.ghost:disabled,
	.alt-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.panel,
	.create-form {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 16px;
	}
	.create-form {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.file-fields {
		display: flex;
		flex-direction: column;
		gap: 10px;
		background: var(--panel-bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 10px 12px;
	}
	.form-intro {
		margin: 0;
		font-size: 12px;
		color: var(--sub);
		line-height: 1.5;
	}
	.hint {
		font-size: 11px;
		color: var(--sub);
		font-weight: 400;
		line-height: 1.4;
	}
	.advanced-toggle {
		align-self: flex-start;
		background: transparent;
		border: none;
		color: var(--sub);
		font-size: 12px;
		cursor: pointer;
		padding: 4px 0;
	}
	.advanced-toggle:hover {
		color: var(--heading);
	}
	.row {
		display: grid;
		gap: 10px;
	}
	.row.two {
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 12px;
		color: var(--sub);
	}
	label > span:first-child {
		color: var(--heading);
		font-weight: 600;
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
	input:disabled {
		opacity: 0.6;
	}
	.form-foot {
		display: flex;
		justify-content: flex-end;
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
	.table-wrap {
		overflow-x: auto;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th,
	td {
		padding: 12px 10px;
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
	.cell-primary {
		font-weight: 600;
	}
	.cell-secondary {
		font-size: 12px;
		color: var(--sub);
	}
	.status-error {
		font-size: 12px;
		color: #f87171;
	}
	.row-actions {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
	}
	.empty {
		color: var(--sub);
		font-style: italic;
		padding: 24px 8px 8px;
	}
</style>

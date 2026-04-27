<script lang="ts">
	import { onMount } from 'svelte';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import Settings2Icon from '@lucide/svelte/icons/settings-2';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import CheckIcon from '@lucide/svelte/icons/check';
	import XIcon from '@lucide/svelte/icons/x';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import type { SkillRecord, SkillCategoryRecord, UpdateSkillPayload } from '$lib/services/skillService';
	import {
		listSkills,
		listSkillCategories,
		updateSkill,
		deleteSkill,
		createSkill
	} from '$lib/services/skillService';

	let {
		darkMode = true,
		initialFilter = 'all',
		openAdd = false
	}: {
		darkMode?: boolean;
		initialFilter?: 'all' | 'candidate' | 'installed' | 'activated';
		openAdd?: boolean;
	} = $props();

	// --- tokens ---
	let pageBg = $derived(darkMode ? '#111827' : '#F2F4F7');
	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.13)' : 'rgba(99,102,241,0.09)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let hoverBg = $derived(darkMode ? 'rgba(45,51,72,0.55)' : 'rgba(228,230,235,0.7)');
	let danger = $derived(darkMode ? '#f87171' : '#dc2626');
	let dangerTint = $derived(darkMode ? 'rgba(248,113,113,0.12)' : 'rgba(220,38,38,0.08)');
	let inputBg = $derived(darkMode ? '#0F1320' : '#F9FAFB');
	let sideBg = $derived(darkMode ? '#161f2b' : '#ECEEF2');
	let successTint = $derived(darkMode ? 'rgba(52,211,153,0.14)' : 'rgba(16,185,129,0.10)');
	let success = $derived(darkMode ? '#34d399' : '#059669');

	// --- list settings persisted in localStorage ---
	const SETTINGS_KEY = 'skill-mgmt-list-settings-v1';

	type ColumnKey = 'name' | 'description' | 'category' | 'status' | 'tags' | 'version' | 'notes' | 'create_time' | 'modify_time';

	type ListSettings = {
		visibleColumns: Record<ColumnKey, boolean>;
		showDelete: boolean;
		showEdit: boolean;
	};

	const defaultSettings: ListSettings = {
		visibleColumns: {
			name: true,
			description: true,
			category: true,
			status: true,
			tags: true,
			version: false,
			notes: false,
			create_time: false,
			modify_time: false
		},
		showDelete: true,
		showEdit: true
	};

	const allColumns: { key: ColumnKey; label: string }[] = [
		{ key: 'name', label: 'Name' },
		{ key: 'description', label: 'Description' },
		{ key: 'category', label: 'Category' },
		{ key: 'status', label: 'Status' },
		{ key: 'tags', label: 'Tags' },
		{ key: 'version', label: 'Version' },
		{ key: 'notes', label: 'Notes' },
		{ key: 'create_time', label: 'Created' },
		{ key: 'modify_time', label: 'Modified' }
	];

	let listSettings = $state<ListSettings>(defaultSettings);
	let settingsDialogOpen = $state(false);
	let draftSettings = $state<ListSettings>(defaultSettings);

	function loadSettings() {
		try {
			const raw = localStorage.getItem(SETTINGS_KEY);
			if (raw) {
				const parsed = JSON.parse(raw) as Partial<ListSettings>;
				listSettings = {
					visibleColumns: { ...defaultSettings.visibleColumns, ...parsed.visibleColumns },
					showDelete: parsed.showDelete ?? defaultSettings.showDelete,
					showEdit: parsed.showEdit ?? defaultSettings.showEdit
				};
			}
		} catch {
			listSettings = { ...defaultSettings };
		}
	}

	function saveSettings() {
		localStorage.setItem(SETTINGS_KEY, JSON.stringify(listSettings));
	}

	function openSettingsDialog() {
		draftSettings = {
			visibleColumns: { ...listSettings.visibleColumns },
			showDelete: listSettings.showDelete,
			showEdit: listSettings.showEdit
		};
		settingsDialogOpen = true;
	}

	function confirmSettings() {
		listSettings = {
			visibleColumns: { ...draftSettings.visibleColumns },
			showDelete: draftSettings.showDelete,
			showEdit: draftSettings.showEdit
		};
		saveSettings();
		settingsDialogOpen = false;
	}

	// --- skills ---
	let skills = $state<SkillRecord[]>([]);
	let skillTotal = $state(0);
	let skillPage = $state(1);
	const skillPageSize = 50;
	let skillLoading = $state(false);
	let skillError = $state('');

	// --- categories ---
	let categories = $state<SkillCategoryRecord[]>([]);

	// active filter
	let statusFilter = $state<'all' | 'candidate' | 'installed' | 'activated'>('all');

	// --- edit state ---
	let editingId = $state<number | null>(null);
	let editDraft = $state<Partial<SkillRecord>>({});
	let editSaving = $state(false);
	let editError = $state('');

	// --- delete state ---
	let deletingId = $state<number | null>(null);
	let deleteConfirmOpen = $state(false);
	let deleteError = $state('');
	let deleteSaving = $state(false);

	// --- add skill state ---
	let addDialogOpen = $state(false);
	let addDraft = $state({ name: '', description: '', category: '', status: 'candidate', tags: '', version: '', notes: '' });
	let addSaving = $state(false);
	let addError = $state('');

	async function loadSkills() {
		skillLoading = true;
		skillError = '';
		try {
			const params: Parameters<typeof listSkills>[0] = {
				page: skillPage,
				pageSize: skillPageSize
			};
			if (statusFilter !== 'all') {
				params.status = statusFilter;
			}
			const res = await listSkills(params);
			skills = res.results ?? [];
			skillTotal = res.total ?? 0;
		} catch (e) {
			skillError = e instanceof Error ? e.message : 'Failed to load skills';
		} finally {
			skillLoading = false;
		}
	}

	function startEdit(s: SkillRecord) {
		editingId = s.id;
		editDraft = {
			name: s.name,
			description: s.description ?? '',
			category: [...(s.category ?? [])],
			status: s.status ?? 'candidate',
			tags: [...(s.tags ?? [])],
			version: s.version ?? '',
			notes: s.notes ?? ''
		};
		editError = '';
	}

	function cancelEdit() {
		editingId = null;
		editError = '';
	}

	async function saveEdit(id: number) {
		const name = (editDraft.name ?? '').trim();
		if (!name) {
			editError = 'Name is required';
			return;
		}
		editSaving = true;
		editError = '';
		try {
			const payload: UpdateSkillPayload = {
				name,
				description: editDraft.description || null,
				category: editDraft.category ?? [],
				status: editDraft.status || null,
				tags: editDraft.tags ?? [],
				version: editDraft.version || null,
				notes: editDraft.notes || null
			};
			await updateSkill(id, payload);
			const idx = skills.findIndex((s) => s.id === id);
			if (idx !== -1) {
				skills[idx] = {
					...skills[idx],
					...payload,
					description: payload.description ?? undefined,
					status: payload.status ?? undefined,
					version: payload.version ?? undefined,
					notes: payload.notes ?? undefined,
					modify_time: new Date().toISOString()
				};
			}
			editingId = null;
		} catch (e) {
			editError = e instanceof Error ? e.message : 'Failed to save';
		} finally {
			editSaving = false;
		}
	}

	function openDeleteConfirm(s: SkillRecord) {
		deletingId = s.id;
		deleteConfirmOpen = true;
		deleteError = '';
	}

	function cancelDelete() {
		deletingId = null;
		deleteConfirmOpen = false;
		deleteError = '';
	}

	async function confirmDelete() {
		if (!deletingId) return;
		deleteSaving = true;
		deleteError = '';
		try {
			await deleteSkill(deletingId);
			skills = skills.filter((s) => s.id !== deletingId);
			skillTotal = Math.max(0, skillTotal - 1);
			deleteConfirmOpen = false;
			deletingId = null;
		} catch (e) {
			deleteError = e instanceof Error ? e.message : 'Failed to delete';
		} finally {
			deleteSaving = false;
		}
	}

	function openAddSkillDialog() {
		addDraft = { name: '', description: '', category: '', status: 'candidate', tags: '', version: '', notes: '' };
		addError = '';
		addDialogOpen = true;
	}

	async function confirmAdd() {
		const name = addDraft.name.trim();
		if (!name) {
			addError = 'Name is required';
			return;
		}
		addSaving = true;
		addError = '';
		try {
			const newSkill = await createSkill({
				name,
				description: addDraft.description.trim() || null,
				category: addDraft.category.split(',').map((s) => s.trim()).filter(Boolean),
				status: addDraft.status || 'candidate',
				tags: addDraft.tags.split(',').map((s) => s.trim()).filter(Boolean),
				version: addDraft.version.trim() || null,
				notes: addDraft.notes.trim() || null
			});
			skills = [newSkill, ...skills];
			skillTotal += 1;
			addDialogOpen = false;
		} catch (e) {
			addError = e instanceof Error ? e.message : 'Failed to create skill';
		} finally {
			addSaving = false;
		}
	}

	function formatDate(v?: string): string {
		if (!v) return '';
		const d = new Date(v);
		if (isNaN(d.getTime())) return v;
		return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
	}

	function formatCategory(cat: string[]): string {
		return cat?.join(' / ') ?? '';
	}

	let visibleCols = $derived(allColumns.filter((c) => listSettings.visibleColumns[c.key]));

	onMount(() => {
		loadSettings();
		statusFilter = initialFilter;
		loadSkills();
		listSkillCategories().then((res) => { categories = res.results ?? []; }).catch(() => {});
		if (openAdd) {
			addDialogOpen = true;
		}
	});
</script>

<div
	class="skill-mgmt-root"
	style="--page:{pageBg}; --side:{sideBg}; --card:{cardBg}; --border:{border}; --accent:{accent}; --accent-tint:{accentTint}; --text-primary:{textPrimary}; --text-secondary:{textSecondary}; --text-muted:{textMuted}; --hover:{hoverBg}; --danger:{danger}; --danger-tint:{dangerTint}; --input:{inputBg}; --success:{success}; --success-tint:{successTint};"
>
	<!-- Main content: skill list -->
	<main class="skill-main">
		<!-- Toolbar -->
		<div class="list-toolbar">
			<div class="list-title-row">
				<h2 class="list-title">
					{statusFilter === 'all' ? 'All Skills' : `${statusFilter.charAt(0).toUpperCase() + statusFilter.slice(1)} Skills`}
				</h2>
				<span class="list-count">{skillTotal} skill{skillTotal === 1 ? '' : 's'}</span>
			</div>
			<div class="toolbar-actions">
				<button
					class="btn-ghost"
					onclick={() => { skillPage = 1; loadSkills(); }}
					title="Refresh"
				>
					<RefreshCwIcon class="w-4 h-4" />
				</button>
				<button class="btn-ghost settings-btn" onclick={openSettingsDialog}>
					<Settings2Icon class="w-4 h-4" />
					Settings
				</button>
				<button class="btn-primary" onclick={openAddSkillDialog}>
					<PlusIcon class="w-4 h-4" />
					Add Skill
				</button>
			</div>
		</div>

		{#if skillError}
			<div class="notice-error">{skillError}</div>
		{/if}

		<!-- Table -->
		<div class="table-wrap">
			{#if skillLoading}
				<div class="table-state">
					<div class="state-glyph">⋯</div>
					<div>Loading skills</div>
				</div>
			{:else if skills.length === 0}
				<div class="table-state">
					<ZapIcon class="w-8 h-8 state-icon" />
					<div class="state-title">No skills found</div>
					<div class="state-copy">No skills in this workspace yet</div>
				</div>
			{:else}
				<table class="skill-table">
					<thead>
						<tr>
							{#each visibleCols as col (col.key)}
								<th class="th" class:th-name={col.key === 'name'}>{col.label}</th>
							{/each}
							{#if listSettings.showEdit || listSettings.showDelete}
								<th class="th th-actions">Actions</th>
							{/if}
						</tr>
					</thead>
					<tbody>
						{#each skills as skill (skill.id)}
							{#if editingId === skill.id}
								<!-- Edit row -->
								<tr class="tr-edit">
									<td colspan={visibleCols.length + (listSettings.showEdit || listSettings.showDelete ? 1 : 0)} style="padding:0;">
										<form
											class="edit-form"
											onsubmit={(e) => { e.preventDefault(); saveEdit(skill.id); }}
										>
											<div class="edit-grid">
												<label class="field">
													<span>Name</span>
													<input bind:value={editDraft.name} placeholder="Skill name" required />
												</label>
												<label class="field">
													<span>Status</span>
													<select bind:value={editDraft.status}>
														<option value="candidate">candidate</option>
														<option value="installed">installed</option>
														<option value="activated">activated</option>
													</select>
												</label>
												<label class="field field-wide">
													<span>Description</span>
													<input bind:value={editDraft.description} placeholder="Brief description" />
												</label>
												<label class="field field-wide">
													<span>Category Path <span class="field-hint">(comma-separated)</span></span>
													<input
														value={editDraft.category?.join(', ') ?? ''}
														oninput={(e) => {
															editDraft.category = (e.currentTarget as HTMLInputElement).value
																.split(',')
																.map((s) => s.trim())
																.filter(Boolean);
														}}
														placeholder="e.g. AI, NLP, Text Processing"
													/>
												</label>
												<label class="field field-wide">
													<span>Pick from existing <span class="field-hint">(optional)</span></span>
													<select
														value=""
														onchange={(e) => {
															const el = e.currentTarget as HTMLSelectElement;
															const cat = categories.find((c) => String(c.id) === el.value);
															if (cat) editDraft.category = [...cat.path];
															el.value = '';
														}}
													>
														<option value="">— select from existing —</option>
														{#each categories as cat (cat.id)}
															<option value={String(cat.id)}>{cat.path.join(' / ')}</option>
														{/each}
													</select>
												</label>
												<label class="field field-wide">
													<span>Tags (comma-separated)</span>
													<input
														value={editDraft.tags?.join(', ') ?? ''}
														oninput={(e) => {
															editDraft.tags = (e.currentTarget as HTMLInputElement).value
																.split(',')
																.map((s) => s.trim())
																.filter(Boolean);
														}}
														placeholder="e.g. nlp, extraction"
													/>
												</label>
												<label class="field">
													<span>Version</span>
													<input bind:value={editDraft.version} placeholder="e.g. 1.0.0" />
												</label>
												<label class="field field-wide">
													<span>Notes</span>
													<input bind:value={editDraft.notes} placeholder="Optional notes" />
												</label>
											</div>
											{#if editError}
												<div class="edit-error">{editError}</div>
											{/if}
											<div class="edit-foot">
												<button class="btn-primary small" type="submit" disabled={editSaving}>
													<CheckIcon class="w-3.5 h-3.5" />
													{editSaving ? 'Saving…' : 'Save'}
												</button>
												<button class="btn-ghost small" type="button" onclick={cancelEdit}>
													<XIcon class="w-3.5 h-3.5" />
													Cancel
												</button>
											</div>
										</form>
									</td>
								</tr>
							{:else}
								<!-- Read row -->
								<tr class="tr-row">
									{#each visibleCols as col (col.key)}
										<td class="td" class:td-name={col.key === 'name'}>
											{#if col.key === 'name'}
												<span class="skill-name">{skill.name}</span>
											{:else if col.key === 'category'}
												<span class="mono-sm">{formatCategory(skill.category)}</span>
											{:else if col.key === 'tags'}
												{#if skill.tags?.length}
													<div class="tag-row">
														{#each skill.tags.slice(0, 3) as tag}
															<span class="tag">{tag}</span>
														{/each}
														{#if skill.tags.length > 3}
															<span class="tag-more">+{skill.tags.length - 3}</span>
														{/if}
													</div>
												{/if}
											{:else if col.key === 'status'}
												{#if skill.status}
													<span
														class="status-badge"
														class:status-activated={skill.status === 'activated'}
														class:status-installed={skill.status === 'installed'}
													>
														{skill.status}
													</span>
												{/if}
											{:else if col.key === 'create_time'}
												<span class="date-val">{formatDate(skill.create_time)}</span>
											{:else if col.key === 'modify_time'}
												<span class="date-val">{formatDate(skill.modify_time)}</span>
											{:else if col.key === 'description'}
												<span class="desc-cell">{skill.description ?? ''}</span>
											{:else}
												{(skill as Record<string, unknown>)[col.key] ?? ''}
											{/if}
										</td>
									{/each}
									{#if listSettings.showEdit || listSettings.showDelete}
										<td class="td td-actions">
											{#if listSettings.showEdit}
												<button class="action-btn" onclick={() => startEdit(skill)} title="Edit">
													<PencilIcon class="w-3.5 h-3.5" />
												</button>
											{/if}
											{#if listSettings.showDelete}
												<button class="action-btn danger-btn" onclick={() => openDeleteConfirm(skill)} title="Delete">
													<Trash2Icon class="w-3.5 h-3.5" />
												</button>
											{/if}
										</td>
									{/if}
								</tr>
							{/if}
						{/each}
					</tbody>
				</table>

				<!-- Pagination -->
				{#if skillTotal > skillPageSize}
					<div class="pagination">
						<span class="page-info">
							{(skillPage - 1) * skillPageSize + 1}–{Math.min(skillPage * skillPageSize, skillTotal)} of {skillTotal}
						</span>
						<div class="page-btns">
							<button
								class="btn-ghost small"
								disabled={skillPage === 1}
								onclick={() => { skillPage--; loadSkills(); }}
							>
								Previous
							</button>
							<button
								class="btn-ghost small"
								disabled={skillPage * skillPageSize >= skillTotal}
								onclick={() => { skillPage++; loadSkills(); }}
							>
								Next
							</button>
						</div>
					</div>
				{/if}
			{/if}
		</div>
	</main>

	<!-- Settings dialog -->
	{#if settingsDialogOpen}
		<div class="dialog-overlay" role="dialog" aria-modal="true" aria-label="List Settings">
			<div class="dialog-panel" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} role="presentation">
				<div class="dialog-header">
					<h3>List Settings</h3>
					<p>Configure which columns and actions are visible in the skills list.</p>
				</div>

				<div class="settings-section">
					<div class="settings-section-title">Columns</div>
					<div class="col-grid">
						{#each allColumns as col (col.key)}
							<label class="col-toggle">
								<input
									type="checkbox"
									bind:checked={draftSettings.visibleColumns[col.key]}
								/>
								<span>{col.label}</span>
							</label>
						{/each}
					</div>
				</div>

				<div class="settings-section">
					<div class="settings-section-title">Operation Columns</div>
					<div class="op-toggles">
						<label class="op-toggle">
							<div>
								<div class="op-label">Delete column</div>
								<div class="op-desc">Show a delete button for each row</div>
							</div>
							<button
								type="button"
								role="switch"
								aria-checked={draftSettings.showDelete}
								onclick={() => (draftSettings.showDelete = !draftSettings.showDelete)}
								class="toggle-switch"
								style="background:{draftSettings.showDelete ? accent : border};"
							>
								<span class="toggle-knob" style="transform:translateX({draftSettings.showDelete ? '22px' : '2px'});"></span>
							</button>
						</label>
						<label class="op-toggle">
							<div>
								<div class="op-label">Edit column</div>
								<div class="op-desc">Show an edit button for each row</div>
							</div>
							<button
								type="button"
								role="switch"
								aria-checked={draftSettings.showEdit}
								onclick={() => (draftSettings.showEdit = !draftSettings.showEdit)}
								class="toggle-switch"
								style="background:{draftSettings.showEdit ? accent : border};"
							>
								<span class="toggle-knob" style="transform:translateX({draftSettings.showEdit ? '22px' : '2px'});"></span>
							</button>
						</label>
					</div>
				</div>

				<div class="dialog-foot">
					<button class="btn-ghost" onclick={() => (settingsDialogOpen = false)}>Cancel</button>
					<button class="btn-primary" onclick={confirmSettings}>Save Settings</button>
				</div>
			</div>
		</div>
	{/if}

	<!-- Add Skill dialog -->
	{#if addDialogOpen}
		<div class="dialog-overlay" role="dialog" aria-modal="true" aria-label="Add Skill">
			<div class="dialog-panel" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} role="presentation">
				<div class="dialog-header">
					<h3>Add Skill</h3>
					<p>Create a new skill in this workspace.</p>
				</div>

				<form onsubmit={(e) => { e.preventDefault(); confirmAdd(); }}>
					<div class="add-grid">
						<label class="add-field">
							<span class="add-label">Name <span class="required-star">*</span></span>
							<input class="add-input" bind:value={addDraft.name} placeholder="Skill name" required />
						</label>
						<label class="add-field">
							<span class="add-label">Status</span>
							<select class="add-input" bind:value={addDraft.status}>
								<option value="candidate">candidate</option>
								<option value="installed">installed</option>
								<option value="activated">activated</option>
							</select>
						</label>
						<label class="add-field add-field-wide">
							<span class="add-label">Description</span>
							<input class="add-input" bind:value={addDraft.description} placeholder="Brief description" />
						</label>
						<label class="add-field add-field-wide">
							<span class="add-label">Category Path <span class="add-hint">(comma-separated, e.g. AI, NLP, Text Processing)</span></span>
							<input class="add-input" bind:value={addDraft.category} placeholder="e.g. AI, NLP, Text Processing" />
						</label>
						<label class="add-field add-field-wide">
							<span class="add-label">Pick from existing <span class="add-hint">(optional — fills Category Path above)</span></span>
							<select
								class="add-input"
								value=""
								onchange={(e) => {
									const el = e.currentTarget as HTMLSelectElement;
									const cat = categories.find((c) => String(c.id) === el.value);
									if (cat) addDraft.category = cat.path.join(', ');
									el.value = '';
								}}
							>
								<option value="">— select from existing —</option>
								{#each categories as cat (cat.id)}
									<option value={String(cat.id)}>{cat.path.join(' / ')}</option>
								{/each}
							</select>
						</label>
						<label class="add-field add-field-wide">
							<span class="add-label">Tags <span class="add-hint">(comma-separated)</span></span>
							<input class="add-input" bind:value={addDraft.tags} placeholder="e.g. nlp, extraction" />
						</label>
						<label class="add-field">
							<span class="add-label">Version</span>
							<input class="add-input" bind:value={addDraft.version} placeholder="e.g. 1.0.0" />
						</label>
						<label class="add-field add-field-wide">
							<span class="add-label">Notes</span>
							<input class="add-input" bind:value={addDraft.notes} placeholder="Optional notes" />
						</label>
					</div>

					{#if addError}
						<div class="notice-error" style="margin-top:12px;">{addError}</div>
					{/if}

					<div class="dialog-foot">
						<button class="btn-ghost" type="button" onclick={() => (addDialogOpen = false)}>Cancel</button>
						<button class="btn-primary" type="submit" disabled={addSaving}>
							<PlusIcon class="w-3.5 h-3.5" />
							{addSaving ? 'Creating…' : 'Create Skill'}
						</button>
					</div>
				</form>
			</div>
		</div>
	{/if}

	<!-- Delete confirm dialog -->
	{#if deleteConfirmOpen}
		{@const target = skills.find((s) => s.id === deletingId)}
		<div class="dialog-overlay" role="dialog" aria-modal="true" aria-label="Confirm Delete">
			<div class="dialog-panel dialog-sm" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} role="presentation">
				<div class="dialog-header">
					<h3>Delete skill?</h3>
					<p>
						Remove <strong>{target?.name}</strong>? This cannot be undone.
					</p>
				</div>
				{#if deleteError}
					<div class="notice-error">{deleteError}</div>
				{/if}
				<div class="dialog-foot">
					<button class="btn-ghost" onclick={cancelDelete}>Cancel</button>
					<button class="btn-danger" onclick={confirmDelete} disabled={deleteSaving}>
						{deleteSaving ? 'Deleting…' : 'Delete'}
					</button>
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.skill-mgmt-root {
		display: flex;
		height: 100%;
		min-height: 0;
		background: var(--page);
		color: var(--text-primary);
		overflow: hidden;
	}

	/* --- Main --- */
	.skill-main {
		flex: 1;
		display: flex;
		flex-direction: column;
		min-width: 0;
		overflow: hidden;
	}

	.list-toolbar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 14px 20px 12px;
		border-bottom: 1px solid var(--border);
		flex-shrink: 0;
		gap: 12px;
	}

	.list-title-row {
		display: flex;
		align-items: baseline;
		gap: 10px;
	}

	.list-title {
		margin: 0;
		font-size: 16px;
		font-weight: 600;
		color: var(--text-primary);
	}

	.list-count {
		font-size: 12px;
		color: var(--text-muted);
	}

	.toolbar-actions {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	/* --- Buttons --- */
	.btn-primary {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		background: var(--accent);
		color: #fff;
		border: none;
		padding: 8px 14px;
		border-radius: 8px;
		font-size: 13px;
		font-weight: 600;
		cursor: pointer;
		transition: opacity 140ms ease;
	}

	.btn-primary:hover {
		opacity: 0.88;
	}

	.btn-primary:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.btn-primary.small {
		padding: 5px 10px;
		font-size: 12px;
	}

	.btn-ghost {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		background: transparent;
		border: 1px solid var(--border);
		color: var(--text-secondary);
		padding: 7px 12px;
		border-radius: 8px;
		font-size: 13px;
		cursor: pointer;
		transition: background 120ms ease, color 120ms ease;
	}

	.btn-ghost:hover {
		background: var(--hover);
		color: var(--text-primary);
	}

	.btn-ghost:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}

	.btn-ghost.small {
		padding: 4px 9px;
		font-size: 12px;
	}

	.btn-danger {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		background: var(--danger-tint);
		border: 1px solid var(--danger);
		color: var(--danger);
		padding: 8px 14px;
		border-radius: 8px;
		font-size: 13px;
		font-weight: 600;
		cursor: pointer;
		transition: opacity 140ms ease;
	}

	.btn-danger:hover {
		opacity: 0.85;
	}

	.btn-danger:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.settings-btn {
		font-size: 13px;
	}

	/* --- Table --- */
	.table-wrap {
		flex: 1;
		overflow: auto;
		scrollbar-width: thin;
		scrollbar-color: var(--border) transparent;
	}

	.skill-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
	}

	.th {
		padding: 10px 14px;
		text-align: left;
		font-size: 11px;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.07em;
		color: var(--text-muted);
		border-bottom: 1px solid var(--border);
		white-space: nowrap;
		background: var(--side);
		position: sticky;
		top: 0;
		z-index: 1;
	}

	.th-name {
		min-width: 160px;
	}

	.th-actions {
		width: 80px;
		text-align: center;
	}

	.td {
		padding: 10px 14px;
		border-bottom: 1px solid var(--border);
		vertical-align: middle;
		color: var(--text-secondary);
	}

	.td-name {
		color: var(--text-primary);
		font-weight: 500;
	}

	.td-actions {
		text-align: center;
		white-space: nowrap;
	}

	.tr-row:hover .td {
		background: var(--hover);
	}

	.tr-edit .td {
		padding: 0;
	}

	.skill-name {
		font-weight: 500;
	}

	.mono-sm {
		font-family: 'Fira Code', 'Cascadia Code', monospace;
		font-size: 11px;
		color: var(--text-muted);
	}

	.desc-cell {
		overflow: hidden;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		-webkit-box-orient: vertical;
		max-width: 280px;
	}

	.date-val {
		font-size: 12px;
		color: var(--text-muted);
		white-space: nowrap;
	}

	.tag-row {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}

	.tag,
	.tag-more {
		display: inline-flex;
		align-items: center;
		padding: 2px 7px;
		border-radius: 999px;
		font-size: 11px;
		background: var(--accent-tint);
		color: var(--accent);
		white-space: nowrap;
	}

	.tag-more {
		color: var(--text-muted);
		background: transparent;
	}

	.status-badge {
		display: inline-flex;
		align-items: center;
		padding: 2px 8px;
		border-radius: 999px;
		font-size: 11px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		background: var(--hover);
		color: var(--text-muted);
	}

	.status-activated {
		background: var(--success-tint);
		color: var(--success);
	}

	.status-installed {
		background: var(--accent-tint);
		color: var(--accent);
	}

	.action-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		background: transparent;
		border: none;
		border-radius: 6px;
		cursor: pointer;
		color: var(--text-muted);
		transition: background 120ms ease, color 120ms ease;
	}

	.action-btn:hover {
		background: var(--hover);
		color: var(--text-primary);
	}

	.danger-btn:hover {
		background: var(--danger-tint);
		color: var(--danger);
	}

	/* --- Edit form --- */
	.edit-form {
		padding: 14px 14px 12px;
		background: var(--card);
		border-bottom: 2px solid var(--accent);
	}

	.edit-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 10px;
		margin-bottom: 10px;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.field span {
		font-size: 11px;
		font-weight: 600;
		color: var(--text-muted);
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}

	.field input,
	.field select {
		background: var(--input);
		color: var(--text-primary);
		border: 1px solid var(--border);
		border-radius: 7px;
		padding: 7px 10px;
		font-size: 13px;
		font-family: inherit;
	}

	.field-wide {
		grid-column: 1 / -1;
	}

	.edit-error {
		background: var(--danger-tint);
		color: var(--danger);
		padding: 7px 10px;
		border-radius: 6px;
		font-size: 12px;
		margin-bottom: 8px;
	}

	.edit-foot {
		display: flex;
		gap: 8px;
		justify-content: flex-end;
	}

	/* --- State panels --- */
	.table-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 10px;
		padding: 64px 24px;
		color: var(--text-muted);
		text-align: center;
	}

	.state-glyph {
		font-size: 28px;
	}

	.state-icon {
		opacity: 0.3;
		color: var(--accent);
	}

	.state-title {
		font-size: 16px;
		font-weight: 500;
		color: var(--text-secondary);
	}

	.state-copy {
		font-size: 13px;
	}

	.notice-error {
		margin: 12px 20px;
		padding: 10px 14px;
		background: var(--danger-tint);
		color: var(--danger);
		border-radius: 8px;
		font-size: 13px;
	}

	/* --- Pagination --- */
	.pagination {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 12px 20px;
		border-top: 1px solid var(--border);
	}

	.page-info {
		font-size: 12px;
		color: var(--text-muted);
	}

	.page-btns {
		display: flex;
		gap: 8px;
	}

	/* --- Dialogs --- */
	.dialog-overlay {
		position: fixed;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 20px;
		background: rgba(10, 15, 30, 0.52);
		backdrop-filter: blur(8px);
		z-index: 40;
	}

	.dialog-panel {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 14px;
		padding: 22px;
		width: min(560px, 100%);
		max-height: calc(100vh - 40px);
		overflow-y: auto;
		box-shadow: 0 24px 60px rgba(4, 10, 24, 0.36);
	}

	.dialog-sm {
		width: min(400px, 100%);
	}

	.dialog-header {
		margin-bottom: 18px;
	}

	.dialog-header h3 {
		margin: 0 0 6px;
		font-size: 17px;
		font-weight: 700;
		color: var(--text-primary);
	}

	.dialog-header p {
		margin: 0;
		font-size: 13px;
		color: var(--text-secondary);
		line-height: 1.55;
	}

	.settings-section {
		margin-bottom: 20px;
	}

	.settings-section-title {
		font-size: 11px;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-muted);
		margin-bottom: 10px;
	}

	.col-grid {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 8px;
	}

	.col-toggle {
		display: flex;
		align-items: center;
		gap: 7px;
		font-size: 13px;
		color: var(--text-secondary);
		cursor: pointer;
		padding: 6px 8px;
		border-radius: 6px;
		transition: background 120ms ease;
	}

	.col-toggle:hover {
		background: var(--hover);
	}

	.col-toggle input[type='checkbox'] {
		accent-color: var(--accent);
		cursor: pointer;
	}

	.op-toggles {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.op-toggle {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		padding: 12px 14px;
		border: 1px solid var(--border);
		border-radius: 10px;
		cursor: pointer;
	}

	.op-label {
		font-size: 13px;
		font-weight: 600;
		color: var(--text-primary);
		margin-bottom: 2px;
	}

	.op-desc {
		font-size: 12px;
		color: var(--text-muted);
	}

	.toggle-switch {
		position: relative;
		flex-shrink: 0;
		width: 48px;
		height: 28px;
		border-radius: 999px;
		border: none;
		cursor: pointer;
		transition: background 180ms ease;
		padding: 0;
	}

	.toggle-knob {
		position: absolute;
		top: 3px;
		width: 22px;
		height: 22px;
		border-radius: 50%;
		background: #fff;
		transition: transform 180ms ease;
	}

	.dialog-foot {
		display: flex;
		gap: 8px;
		justify-content: flex-end;
		margin-top: 18px;
	}

	/* --- Add Skill form fields --- */
	.add-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 14px;
	}

	.add-field {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.add-field-wide {
		grid-column: 1 / -1;
	}

	.add-label {
		font-size: 12px;
		font-weight: 600;
		color: var(--text-secondary);
		letter-spacing: 0.02em;
	}

	.add-hint,
	.field-hint {
		font-weight: 400;
		color: var(--text-muted);
	}

	.required-star {
		color: var(--danger);
	}

	.add-input {
		width: 100%;
		background: var(--input);
		color: var(--text-primary);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 12px;
		font-size: 13px;
		font-family: inherit;
		outline: none;
		transition: border-color 140ms ease;
		box-sizing: border-box;
	}

	.add-input:focus {
		border-color: var(--accent);
	}

	.add-input::placeholder {
		color: var(--text-muted);
	}
</style>

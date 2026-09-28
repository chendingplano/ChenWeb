<script lang="ts">
	import { onMount } from 'svelte';
	import {
		createRelease,
		deleteRelease,
		listReleases,
		updateRelease,
		type Release,
		type ReleaseInput,
		type ReleaseItem
	} from './releases-client';

	let { darkMode = true }: { darkMode?: boolean } = $props();
	const colors = $derived({
		bg: darkMode ? '#171B26' : '#F2F4F7',
		card: darkMode ? '#1F2333' : '#fff',
		text: darkMode ? '#E2E8F0' : '#111827',
		muted: darkMode ? '#94A3B8' : '#64748B',
		border: darkMode ? '#2D3348' : '#DCE2EA'
	});
	const today = () => {
		const d = new Date();
		return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
	};
	const empty = (): ReleaseInput => ({
		major_version: '',
		minor_version: '',
		release_notes: '',
		release_date: today(),
		items: []
	});
	const emptyItem = (): ReleaseItem => ({
		item_type: 'new feature',
		description: '',
		notes: '',
		pull_request: '',
		ticket_num: ''
	});
	let releases = $state<Release[]>([]);
	let draft = $state<ReleaseInput>(empty());
	let editingID = $state<number | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let formError = $state('');
	let expandedID = $state<number | null>(null);

	async function load() {
		loading = true;
		error = '';
		try {
			releases = await listReleases();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Unable to load releases.';
		} finally {
			loading = false;
		}
	}

	function newRelease() {
		editingID = null;
		draft = empty();
		formError = '';
	}
	function edit(r: Release) {
		editingID = r.id;
		draft = {
			major_version: r.major_version,
			minor_version: r.minor_version,
			release_notes: r.release_notes,
			release_date: r.release_date,
			items: r.items.map((item) => ({ ...item }))
		};
		formError = '';
		window.scrollTo({ top: 0, behavior: 'smooth' });
	}

	async function save() {
		if (!draft.major_version.trim() || !draft.minor_version.trim() || !draft.release_date) {
			formError = 'Major version, minor version, and release date are required.';
			return;
		}
		if (draft.items.some((item) => !item.description.trim())) {
			formError = 'Every released item needs a description.';
			return;
		}
		saving = true;
		formError = '';
		try {
			const input = {
				...draft,
				major_version: draft.major_version.trim(),
				minor_version: draft.minor_version.trim()
			};
			if (editingID === null) await createRelease(input);
			else await updateRelease(editingID, input);
			newRelease();
			await load();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Unable to save release.';
		} finally {
			saving = false;
		}
	}

	async function remove(r: Release) {
		if (!confirm(`Delete release ${r.major_version}.${r.minor_version} and all of its items?`))
			return;
		try {
			await deleteRelease(r.id);
			if (editingID === r.id) newRelease();
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Unable to delete release.';
		}
	}

	onMount(load);
</script>

<div class="page" style={`background:${colors.bg};color:${colors.text};`}>
	<section class="panel" style={`background:${colors.card};border-color:${colors.border};`}>
		<div class="heading">
			<div>
				<span class="eyebrow" style={`color:${colors.muted}`}>SYSTEM ADMIN / SYSTEM</span>
				<h1>Releases</h1>
				<p style={`color:${colors.muted}`}>
					Record a version, release date, notes, and the work included in it.
				</p>
			</div>
			<button class="secondary" onclick={newRelease}>New release</button>
		</div>
		<h2>{editingID === null ? 'Create release' : 'Modify release'}</h2>
		<div class="fields version-fields">
			<label>Major version<input bind:value={draft.major_version} placeholder="12" /></label>
			<label>Minor version<input bind:value={draft.minor_version} placeholder="2" /></label>
			<label>Release date<input type="date" bind:value={draft.release_date} /></label>
		</div>
		<label class="full"
			>Release notes<textarea
				bind:value={draft.release_notes}
				rows="3"
				placeholder="Summary of this release"
			></textarea></label
		>
		<div class="item-heading">
			<h3>Released items</h3>
			<button class="secondary" onclick={() => (draft.items = [...draft.items, emptyItem()])}
				>Add item</button
			>
		</div>
		{#each draft.items as item, i (item)}
			<div class="item-form" style={`border-color:${colors.border}`}>
				<div class="item-title">
					<strong>Item {i + 1}</strong><button
						class="text danger"
						onclick={() => (draft.items = draft.items.filter((_, index) => index !== i))}
						>Remove</button
					>
				</div>
				<div class="fields">
					<label
						>Type<select bind:value={item.item_type}
							><option value="bug fix">Bug fix</option><option value="improvement"
								>Improvement</option
							><option value="new feature">New feature</option></select
						></label
					><label>Ticket number<input bind:value={item.ticket_num} /></label><label
						>Pull request<input bind:value={item.pull_request} placeholder="Number or URL" /></label
					>
				</div>
				<label class="full">Description<input bind:value={item.description} /></label>
				<label class="full">Notes<textarea bind:value={item.notes} rows="2"></textarea></label>
			</div>
		{/each}
		{#if draft.items.length === 0}<p class="muted" style={`color:${colors.muted}`}>
				No released items yet.
			</p>{/if}
		{#if formError}<p class="error" role="alert">{formError}</p>{/if}
		<div class="form-actions">
			<button onclick={save} disabled={saving}
				>{saving ? 'Saving…' : editingID === null ? 'Create release' : 'Save changes'}</button
			>{#if editingID !== null}<button class="secondary" onclick={newRelease}>Cancel</button>{/if}
		</div>
	</section>

	<section class="panel" style={`background:${colors.card};border-color:${colors.border};`}>
		<div class="heading">
			<div>
				<h2>All releases</h2>
				<p style={`color:${colors.muted}`}>Newest major and minor versions first</p>
			</div>
			<button class="secondary" onclick={load}>Refresh</button>
		</div>
		{#if loading}<p class="muted">Loading releases…</p>{:else if error}<p
				class="error"
				role="alert"
			>
				{error}
			</p>{:else if releases.length === 0}<p class="muted">No releases have been created.</p>{:else}
			<div class="table-wrap">
				<table>
					<thead
						><tr
							><th>Version</th><th>Release date</th><th>Release notes</th><th>Items</th><th
								>Actions</th
							></tr
						></thead
					><tbody>
						{#each releases as r (r.id)}
							<tr
								><td
									><button
										class="text version"
										onclick={() => (expandedID = expandedID === r.id ? null : r.id)}
										aria-expanded={expandedID === r.id}>{r.major_version}.{r.minor_version}</button
									></td
								><td>{r.release_date}</td><td class="notes-preview">{r.release_notes || '—'}</td><td
									>{r.items.length}</td
								><td class="actions"
									><button class="text" onclick={() => edit(r)}>Edit</button><button
										class="text danger"
										onclick={() => remove(r)}>Delete</button
									></td
								></tr
							>
							{#if expandedID === r.id}<tr
									><td colspan="5"
										><div class="details">
											<h3>Release {r.major_version}.{r.minor_version}</h3>
											<p class="release-notes">{r.release_notes || 'No release notes.'}</p>
											{#each r.items as item (item.id)}<div
													class="detail-item"
													style={`border-color:${colors.border}`}
												>
													<strong>{item.item_type}</strong>
													<p>{item.description}</p>
													{#if item.notes}<p class="muted">{item.notes}</p>{/if}<small
														>{item.ticket_num
															? `Ticket: ${item.ticket_num}`
															: ''}{item.ticket_num && item.pull_request
															? ' · '
															: ''}{item.pull_request
															? `Pull request: ${item.pull_request}`
															: ''}</small
													>
												</div>{:else}<p class="muted">No released items.</p>{/each}
										</div></td
									></tr
								>{/if}
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</section>
</div>

<style>
	.page {
		min-height: 100%;
		padding: 24px;
		display: grid;
		align-content: start;
		gap: 18px;
		font: 14px system-ui;
	}
	.panel {
		border: 1px solid;
		border-radius: 12px;
		padding: 22px;
		box-shadow: 0 4px 18px #0001;
	}
	.heading,
	.item-heading,
	.item-title,
	.form-actions,
	.actions {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 12px;
	}
	.heading {
		margin-bottom: 20px;
	}
	.heading h1 {
		font-size: 24px;
		margin: 5px 0;
	}
	.heading h2,
	h2,
	h3 {
		margin: 0;
	}
	.heading p {
		margin: 5px 0 0;
	}
	.eyebrow {
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.12em;
	}
	.fields {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 12px;
		margin: 15px 0;
	}
	.version-fields {
		max-width: 780px;
	}
	label {
		display: grid;
		gap: 6px;
		font-weight: 600;
		font-size: 12px;
	}
	.full {
		margin: 12px 0;
	}
	input,
	select,
	textarea {
		width: 100%;
		min-width: 0;
		border: 1px solid #64748b88;
		border-radius: 7px;
		padding: 9px;
		background: transparent;
		color: inherit;
		font: inherit;
	}
	textarea {
		resize: vertical;
	}
	button {
		border: 0;
		border-radius: 7px;
		padding: 8px 12px;
		background: #6366f1;
		color: white;
		cursor: pointer;
		font: inherit;
	}
	button:disabled {
		opacity: 0.55;
		cursor: wait;
	}
	.secondary {
		background: transparent;
		border: 1px solid #64748b88;
		color: inherit;
	}
	.text {
		background: transparent;
		color: #818cf8;
		padding: 4px;
	}
	.danger {
		color: #ef4444;
	}
	.item-heading {
		margin-top: 24px;
	}
	.item-form {
		border: 1px solid;
		border-radius: 9px;
		margin: 12px 0;
		padding: 14px;
	}
	.item-title strong {
		font-size: 13px;
	}
	.form-actions {
		justify-content: flex-start;
		margin-top: 20px;
	}
	.table-wrap {
		overflow: auto;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th,
	td {
		text-align: left;
		padding: 11px;
		border-bottom: 1px solid #64748b44;
		vertical-align: top;
	}
	th {
		font-size: 12px;
		white-space: nowrap;
	}
	.version {
		font-weight: 700;
		font-family: ui-monospace, monospace;
	}
	.notes-preview {
		max-width: 420px;
		white-space: pre-wrap;
	}
	.actions {
		justify-content: flex-start;
		white-space: nowrap;
	}
	.details {
		padding: 12px 8px;
	}
	.release-notes,
	.detail-item p {
		white-space: pre-wrap;
	}
	.detail-item {
		border-top: 1px solid;
		padding: 12px 0;
	}
	.detail-item p {
		margin: 6px 0;
	}
	.detail-item small,
	.muted {
		color: #94a3b8;
	}
	.error {
		color: #ef4444;
	}
	.page :global(button:focus-visible),
	.page :global(input:focus-visible),
	.page :global(textarea:focus-visible),
	.page :global(select:focus-visible) {
		outline: 2px solid #818cf8;
		outline-offset: 2px;
	}
	@media (max-width: 760px) {
		.page {
			padding: 12px;
		}
		.panel {
			padding: 16px;
		}
		.fields {
			grid-template-columns: 1fr;
		}
		.heading {
			align-items: flex-start;
		}
		.table-wrap {
			font-size: 12px;
		}
	}
</style>

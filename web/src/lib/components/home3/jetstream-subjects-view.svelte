<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { onMount } from 'svelte';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import CircleCheckBigIcon from '@lucide/svelte/icons/circle-check-big';

	let { darkMode = true }: { darkMode: boolean } = $props();

	type StoredSubject = {
		id: number;
		subject: string;
		description?: string | null;
		payload_format?: string | null;
		is_active: boolean;
		created_by?: string | null;
		created_at: string;
		updated_at: string;
	};

	type FormMode = 'create' | 'edit';

	let subjects = $state<StoredSubject[]>([]);
	let loading = $state(false);
	let error = $state('');
	let success = $state('');

	let showFormDialog = $state(false);
	let showDeleteDialog = $state(false);
	let showHelp = $state(false);
	let formMode = $state<FormMode>('create');
	let formSubjectId = $state<number | null>(null);
	let formSubject = $state('');
	let formDescription = $state('');
	let formPayloadFormat = $state('');
	let formIsActive = $state(true);
	let subjectToDelete = $state<StoredSubject | null>(null);

	let saving = $state(false);
	let deleting = $state(false);

	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let danger = $derived(darkMode ? '#F87171' : '#DC2626');
	let ok = $derived(darkMode ? '#34D399' : '#059669');

	function fmtDate(value: string): string {
		const d = new Date(value);
		return Number.isNaN(d.getTime()) ? value : d.toLocaleString();
	}

	async function loadSubjects() {
		loading = true;
		error = '';
		try {
			const res = await fetch('/api/v1/jetstream/nats-subjects', { credentials: 'same-origin' });
			const data = await res.json().catch(() => ({}));
			if (!res.ok || !data.ok) {
				throw new Error(
					data.message ?? m.jetstream_subjects_failed_to_load_subjects({ status: res.status })
				);
			}
			subjects = Array.isArray(data.subjects) ? data.subjects : [];
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
			subjects = [];
		} finally {
			loading = false;
		}
	}

	function openCreateDialog() {
		success = '';
		error = '';
		formMode = 'create';
		formSubjectId = null;
		formSubject = '';
		formDescription = '';
		formPayloadFormat = '';
		formIsActive = true;
		showHelp = false;
		showFormDialog = true;
	}

	function openEditDialog(s: StoredSubject) {
		success = '';
		error = '';
		formMode = 'edit';
		formSubjectId = s.id;
		formSubject = s.subject ?? '';
		formDescription = s.description ?? '';
		formPayloadFormat = s.payload_format ?? '';
		formIsActive = !!s.is_active;
		showHelp = false;
		showFormDialog = true;
	}

	function closeFormDialog() {
		showFormDialog = false;
		showHelp = false;
	}

	function openDeleteDialog(s: StoredSubject) {
		success = '';
		error = '';
		subjectToDelete = s;
		showDeleteDialog = true;
	}

	function closeDeleteDialog() {
		subjectToDelete = null;
		showDeleteDialog = false;
	}

	async function handleSave() {
		error = '';
		success = '';
		if (!formSubject.trim()) {
			error = m.jetstream_subjects_subject_is_required();
			return;
		}

		saving = true;
		try {
			if (formMode === 'create') {
				const res = await fetch('/api/v1/jetstream/nats-subjects', {
					method: 'POST',
					credentials: 'same-origin',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({
						subject: formSubject.trim(),
						description: formDescription.trim(),
						payload_format: formPayloadFormat.trim()
					})
				});
				const data = await res.json().catch(() => ({}));
				if (!res.ok || !data.ok) {
					throw new Error(
						data.message ?? m.jetstream_subjects_failed_to_create_subject({ status: res.status })
					);
				}
				success = m.jetstream_subjects_subject_created();
			} else {
				if (formSubjectId == null) {
					throw new Error(m.jetstream_subjects_invalid_subject_id());
				}
				const res = await fetch(`/api/v1/jetstream/nats-subjects/${formSubjectId}`, {
					method: 'PUT',
					credentials: 'same-origin',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({
						subject: formSubject.trim(),
						description: formDescription.trim(),
						payload_format: formPayloadFormat.trim(),
						is_active: formIsActive
					})
				});
				const data = await res.json().catch(() => ({}));
				if (!res.ok || !data.ok) {
					throw new Error(
						data.message ?? m.jetstream_subjects_failed_to_update_subject({ status: res.status })
					);
				}
				success = m.jetstream_subjects_subject_updated();
			}
			closeFormDialog();
			await loadSubjects();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			saving = false;
		}
	}

	async function confirmDelete() {
		error = '';
		success = '';
		if (!subjectToDelete) {
			error = m.jetstream_subjects_no_subject_selected_for_deletion();
			return;
		}
		deleting = true;
		try {
			const res = await fetch(`/api/v1/jetstream/nats-subjects/${subjectToDelete.id}`, {
				method: 'DELETE',
				credentials: 'same-origin'
			});
			const data = await res.json().catch(() => ({}));
			if (!res.ok || !data.ok) {
				throw new Error(
					data.message ?? m.jetstream_subjects_failed_to_delete_subject({ status: res.status })
				);
			}
			success = m.jetstream_subjects_subject_deleted();
			closeDeleteDialog();
			await loadSubjects();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			deleting = false;
		}
	}

	onMount(() => {
		loadSubjects();
	});
</script>

<div class="space-y-4 p-6">
	<div class="rounded-xl p-5" style="background:{cardBg}; border:1px solid {borderColor};">
		<div class="flex items-start justify-between gap-3">
			<div>
				<h2 style="font-size:18px; font-weight:600; color:{textPrimary};">
					{m.jetstream_subjects_subjects()}
				</h2>
				<p style="font-size:13px; color:{textSecondary};">
					{m.jetstream_subjects_manage_subjects_stored_in_shared()}
				</p>
			</div>
			<div class="flex items-center gap-2">
				<button
					onclick={openCreateDialog}
					class="inline-flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2"
					style="background:{accent}; color:white; border:none;"
				>
					<PlusIcon class="h-4 w-4" />
					{m.jetstream_subjects_create()}
				</button>
				<button
					onclick={loadSubjects}
					disabled={loading}
					class="inline-flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2"
					style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				>
					<RefreshCwIcon class="h-4 w-4" />
					{m.jetstream_subjects_refresh()}
				</button>
			</div>
		</div>
	</div>

	{#if error}
		<div
			class="flex items-start gap-2 rounded-xl p-4"
			style="background:{danger}20; border:1px solid {danger}70; color:{danger};"
		>
			<CircleAlertIcon class="mt-0.5 h-4 w-4" />
			<div style="font-size:13px;">{error}</div>
		</div>
	{/if}

	{#if success}
		<div
			class="flex items-start gap-2 rounded-xl p-4"
			style="background:{ok}20; border:1px solid {ok}70; color:{ok};"
		>
			<CircleCheckBigIcon class="mt-0.5 h-4 w-4" />
			<div style="font-size:13px;">{success}</div>
		</div>
	{/if}

	<div
		class="overflow-auto rounded-xl p-4"
		style="background:{cardBg}; border:1px solid {borderColor};"
	>
		<table style="width:100%; border-collapse:collapse;">
			<thead>
				<tr style="color:{textSecondary}; font-size:12px;">
					<th style="text-align:left; padding:8px 10px; border-bottom:1px solid {borderColor};"
						>{m.jetstream_subjects_subject()}</th
					>
					<th style="text-align:left; padding:8px 10px; border-bottom:1px solid {borderColor};"
						>{m.jetstream_subjects_description()}</th
					>
					<th style="text-align:left; padding:8px 10px; border-bottom:1px solid {borderColor};"
						>{m.jetstream_subjects_payload_format()}</th
					>
					<th style="text-align:left; padding:8px 10px; border-bottom:1px solid {borderColor};"
						>{m.jetstream_subjects_active()}</th
					>
					<th style="text-align:left; padding:8px 10px; border-bottom:1px solid {borderColor};"
						>{m.jetstream_subjects_created_by()}</th
					>
					<th style="text-align:left; padding:8px 10px; border-bottom:1px solid {borderColor};"
						>{m.jetstream_subjects_created_at()}</th
					>
					<th style="text-align:left; padding:8px 10px; border-bottom:1px solid {borderColor};"
						>{m.jetstream_subjects_actions()}</th
					>
				</tr>
			</thead>
			<tbody>
				{#if subjects.length === 0}
					<tr>
						<td colspan="7" style="padding:12px 10px; color:{textSecondary};">
							{loading
								? m.jetstream_subjects_loading_subjects()
								: m.jetstream_subjects_no_subjects_yet()}
						</td>
					</tr>
				{:else}
					{#each subjects as s}
						<tr>
							<td
								style="padding:10px; border-bottom:1px solid {borderColor}; color:{textPrimary}; font-family:monospace;"
								>{s.subject}</td
							>
							<td
								style="padding:10px; border-bottom:1px solid {borderColor}; color:{textSecondary};"
								>{s.description ?? '-'}</td
							>
							<td
								style="padding:10px; border-bottom:1px solid {borderColor}; color:{textSecondary}; white-space:pre-wrap;"
								>{s.payload_format ?? '-'}</td
							>
							<td
								style="padding:10px; border-bottom:1px solid {borderColor}; color:{textSecondary};"
								>{s.is_active ? 'yes' : 'no'}</td
							>
							<td
								style="padding:10px; border-bottom:1px solid {borderColor}; color:{textSecondary};"
								>{s.created_by ?? '-'}</td
							>
							<td
								style="padding:10px; border-bottom:1px solid {borderColor}; color:{textSecondary};"
								>{fmtDate(s.created_at)}</td
							>
							<td style="padding:10px; border-bottom:1px solid {borderColor};">
								<div class="flex items-center gap-2">
									<button
										onclick={() => openEditDialog(s)}
										class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs"
										style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
									>
										{m.jetstream_subjects_edit()}
									</button>
									<button
										onclick={() => openDeleteDialog(s)}
										class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs"
										style="background:{danger}; color:white; border:none;"
									>
										{m.jetstream_subjects_delete()}
									</button>
								</div>
							</td>
						</tr>
					{/each}
				{/if}
			</tbody>
		</table>
	</div>
</div>

{#if showFormDialog}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center p-4"
		style="background:rgba(0,0,0,0.45);"
	>
		<div
			class="w-full max-w-2xl rounded-xl p-5"
			style="background:{cardBg}; border:1px solid {borderColor};"
		>
			<h3 style="font-size:18px; font-weight:600; color:{textPrimary};">
				{formMode === 'create'
					? m.jetstream_subjects_create_subject()
					: m.jetstream_subjects_edit_subject()}
			</h3>
			<p style="font-size:12px; color:{textSecondary}; margin-top:4px;">
				{formMode === 'create'
					? m.jetstream_subjects_create_a_new_nats_subject()
					: m.jetstream_subjects_update_subject({ formSubjectId: formSubjectId ?? '-' })}
			</p>

			<div class="mt-4 grid gap-3">
				<label class="grid gap-1.5">
					<span style="font-size:12px; color:{textSecondary};"
						>{m.jetstream_subjects_subject()}</span
					>
					<input
						bind:value={formSubject}
						placeholder={m.jetstream_subjects_e_g_kb_pdf_parsed()}
						style="height:36px; border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:0 10px;"
					/>
				</label>
				<label class="grid gap-1.5">
					<span style="font-size:12px; color:{textSecondary};"
						>{m.jetstream_subjects_description_optional()}</span
					>
					<textarea
						bind:value={formDescription}
						rows="3"
						placeholder={m.jetstream_subjects_what_this_subject_is_used()}
						style="border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:10px; resize:vertical;"
					></textarea>
				</label>
				<label class="grid gap-1.5">
					<span style="font-size:12px; color:{textSecondary};"
						>{m.jetstream_subjects_payload_format_optional()}</span
					>
					<textarea
						bind:value={formPayloadFormat}
						rows="5"
						placeholder={m.jetstream_subjects_e_g_record_id_int64()}
						style="border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:10px; resize:vertical;"
					></textarea>
				</label>
				{#if formMode === 'edit'}
					<label
						class="inline-flex items-center gap-2"
						style="color:{textSecondary}; font-size:13px;"
					>
						<input type="checkbox" bind:checked={formIsActive} />
						{m.jetstream_subjects_active()}
					</label>
				{/if}
			</div>

			{#if showHelp}
				<div
					class="mt-4 rounded-lg p-3"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textSecondary};"
				>
					<div style="font-size:12px;">
						{m.jetstream_subjects_payload_format_can_be_plain()}
					</div>
					<pre
						style="margin-top:8px; font-size:12px; color:{textPrimary}; white-space:pre-wrap;">record_id:int64
file_name:string
status:string</pre>
				</div>
			{/if}

			<div class="mt-5 flex items-center justify-end gap-2">
				<button
					onclick={() => {
						showHelp = !showHelp;
					}}
					class="inline-flex cursor-pointer items-center gap-1 rounded-lg px-3 py-2"
					style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				>
					{m.jetstream_subjects_help()}
				</button>
				<button
					onclick={handleSave}
					disabled={saving}
					class="inline-flex cursor-pointer items-center gap-1 rounded-lg px-3 py-2"
					style="background:{accent}; color:white; border:none;"
				>
					{saving ? m.jetstream_subjects_saving() : m.jetstream_subjects_save()}
				</button>
				<button
					onclick={closeFormDialog}
					disabled={saving}
					class="inline-flex cursor-pointer items-center gap-1 rounded-lg px-3 py-2"
					style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				>
					{m.jetstream_subjects_cancel()}
				</button>
			</div>
		</div>
	</div>
{/if}

{#if showDeleteDialog && subjectToDelete}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center p-4"
		style="background:rgba(0,0,0,0.45);"
	>
		<div
			class="w-full max-w-md rounded-xl p-5"
			style="background:{cardBg}; border:1px solid {borderColor};"
		>
			<h3 style="font-size:18px; font-weight:600; color:{textPrimary};">
				{m.jetstream_subjects_confirm_delete()}
			</h3>
			<p style="font-size:13px; color:{textSecondary}; margin-top:8px;">
				{m.jetstream_subjects_are_you_sure_you_want()}
				<span style="font-family:monospace; color:{textPrimary};">{subjectToDelete.subject}</span>?
			</p>
			<div class="mt-5 flex items-center justify-end gap-2">
				<button
					onclick={closeDeleteDialog}
					disabled={deleting}
					class="inline-flex cursor-pointer items-center gap-1 rounded-lg px-3 py-2"
					style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				>
					{m.jetstream_subjects_cancel()}
				</button>
				<button
					onclick={confirmDelete}
					disabled={deleting}
					class="inline-flex cursor-pointer items-center gap-1 rounded-lg px-3 py-2"
					style="background:{danger}; color:white; border:none;"
				>
					{deleting ? m.jetstream_subjects_deleting() : m.jetstream_subjects_delete()}
				</button>
			</div>
		</div>
	</div>
{/if}

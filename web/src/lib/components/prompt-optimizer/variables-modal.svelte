<script lang="ts">
	import XIcon from '@lucide/svelte/icons/x';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import {
		listVariables,
		upsertVariable,
		deleteVariable,
		type Variable
	} from './api';

	let {
		darkMode = true,
		open = false,
		onClose
	}: {
		darkMode?: boolean;
		open?: boolean;
		onClose: () => void;
	} = $props();

	let cardBg        = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2      = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor   = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent        = $derived(darkMode ? '#818CF8' : '#6366F1');
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let danger        = $derived(darkMode ? '#F87171' : '#DC2626');
	let dangerTint    = $derived(darkMode ? 'rgba(248,113,113,0.12)' : 'rgba(220,38,38,0.08)');

	let vars      = $state<Variable[]>([]);
	let loading   = $state(false);
	let loadError = $state('');

	let fName    = $state('');
	let fValue   = $state('');
	let fError   = $state('');
	let saving   = $state(false);

	let lastOpen = false;
	$effect(() => {
		if (open && !lastOpen) {
			void refresh();
		} else if (!open && lastOpen) {
			fName = '';
			fValue = '';
			fError = '';
		}
		lastOpen = open;
	});

	async function refresh() {
		loading = true;
		loadError = '';
		try {
			const list = await listVariables();
			list.sort((a, b) => a.name.localeCompare(b.name));
			vars = list;
		} catch (e) {
			loadError = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	const VALID_NAME = /^[a-zA-Z_][a-zA-Z0-9_]*$/;

	async function save() {
		fError = '';
		const name = fName.trim();
		if (!name) { fError = 'Name is required'; return; }
		if (!VALID_NAME.test(name)) {
			fError = 'Name must start with a letter or underscore and contain only letters, digits, and underscores';
			return;
		}
		saving = true;
		try {
			await upsertVariable({ name, value: fValue });
			fName = '';
			fValue = '';
			await refresh();
		} catch (e) {
			fError = e instanceof Error ? e.message : String(e);
		} finally {
			saving = false;
		}
	}

	async function remove(v: Variable) {
		if (!confirm(`Delete variable "${v.name}"?`)) return;
		try {
			await deleteVariable(v.id);
			await refresh();
		} catch (e) {
			loadError = e instanceof Error ? e.message : String(e);
		}
	}

	function backdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget) onClose();
	}
</script>

{#if open}
	<div
		role="presentation"
		class="fixed inset-0 flex items-center justify-center"
		style="background:rgba(0,0,0,0.5); z-index:1000;"
		onclick={backdropClick}
	>
		<div
			class="rounded-xl flex flex-col"
			style="background:{cardBg}; border:1px solid {borderColor}; width:min(640px, 92vw); max-height:88vh;"
		>
			<div
				class="flex items-center justify-between px-6 py-4"
				style="border-bottom:1px solid {borderColor};"
			>
				<div>
					<h2 style="font-size:16px; font-weight:600; color:{textPrimary};">Variables</h2>
					<p style="font-size:12px; color:{textMuted}; margin-top:2px;">
						Reusable values referenced as <code style="color:{textSecondary};">{'{{name}}'}</code> inside prompts.
					</p>
				</div>
				<button
					onclick={onClose}
					aria-label="Close"
					class="rounded-lg p-1.5 cursor-pointer"
					style="background:transparent; border:none; color:{textSecondary};"
				>
					<XIcon class="w-4 h-4" />
				</button>
			</div>

			<div class="flex-1 overflow-y-auto px-6 py-4" style="scrollbar-width:thin; scrollbar-color:{borderColor} transparent;">
				{#if loadError}
					<div
						class="rounded-lg px-3 py-2 mb-3"
						style="background:{dangerTint}; color:{danger}; font-size:13px;"
					>
						{loadError}
					</div>
				{/if}

				<div
					class="rounded-lg px-4 py-3 mb-4"
					style="background:{surface2}; border:1px solid {borderColor};"
				>
					<div class="grid grid-cols-1 gap-2">
						<div>
							<label for="vm-name" style="font-size:12px; color:{textSecondary}; font-weight:500;">Name</label>
							<input
								id="vm-name"
								type="text"
								bind:value={fName}
								placeholder="e.g. company_name"
								class="w-full rounded-lg px-3 py-2 mt-1"
								style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
							/>
						</div>
						<div>
							<label for="vm-value" style="font-size:12px; color:{textSecondary}; font-weight:500;">Value</label>
							<textarea
								id="vm-value"
								bind:value={fValue}
								rows="3"
								placeholder="Value — can be multi-line"
								class="w-full rounded-lg px-3 py-2 mt-1"
								style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
							></textarea>
						</div>
					</div>

					{#if fError}
						<div
							class="rounded-lg px-3 py-2 mt-2"
							style="background:{dangerTint}; color:{danger}; font-size:13px;"
						>
							{fError}
						</div>
					{/if}

					<div class="flex justify-end mt-2">
						<button
							onclick={save}
							disabled={saving}
							class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 cursor-pointer"
							style="background:{accent}; color:white; border:none; font-size:13px; font-weight:600; opacity:{saving ? 0.6 : 1};"
						>
							<PlusIcon class="w-3.5 h-3.5" />
							{saving ? 'Saving…' : 'Save variable'}
						</button>
					</div>
				</div>

				{#if loading}
					<div style="color:{textMuted}; font-size:13px;">Loading…</div>
				{:else if vars.length === 0}
					<div style="color:{textMuted}; font-size:13px;" class="text-center py-4">
						No variables yet.
					</div>
				{:else}
					<div class="space-y-2">
						{#each vars as v (v.id)}
							<div
								class="rounded-lg px-4 py-3 flex items-start justify-between gap-3"
								style="background:{surface2}; border:1px solid {borderColor};"
							>
								<div class="flex-1 min-w-0">
									<div style="font-size:13px; font-weight:600; color:{textPrimary}; font-family:ui-monospace, SFMono-Regular, Menlo, monospace;">
										{v.name}
									</div>
									<div
										style="font-size:12px; color:{textSecondary}; margin-top:2px; white-space:pre-wrap; word-break:break-word;"
									>
										{v.value}
									</div>
								</div>
								<button
									onclick={() => remove(v)}
									aria-label="Delete"
									class="rounded-lg p-1.5 cursor-pointer flex-shrink-0"
									style="background:transparent; color:{danger}; border:1px solid {borderColor};"
								>
									<Trash2Icon class="w-3.5 h-3.5" />
								</button>
							</div>
						{/each}
					</div>
				{/if}
			</div>

			<div
				class="flex items-center justify-end gap-2 px-6 py-3"
				style="border-top:1px solid {borderColor};"
			>
				<button
					onclick={onClose}
					class="rounded-lg px-4 py-2 cursor-pointer"
					style="background:transparent; color:{textSecondary}; border:1px solid {borderColor}; font-size:13px;"
				>
					Close
				</button>
			</div>
		</div>
	</div>
{/if}

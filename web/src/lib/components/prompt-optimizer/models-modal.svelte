<script lang="ts">
	import XIcon from '@lucide/svelte/icons/x';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import {
		listModels,
		createModel,
		updateModel,
		deleteModel,
		type Model,
		type Provider,
		type CreateModelRequest
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
	let accentTint    = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let danger        = $derived(darkMode ? '#F87171' : '#DC2626');
	let dangerTint    = $derived(darkMode ? 'rgba(248,113,113,0.12)' : 'rgba(220,38,38,0.08)');

	const providerOptions: { value: Provider; label: string }[] = [
		{ value: 'openai',              label: 'OpenAI' },
		{ value: 'openai_compatible',   label: 'OpenAI-compatible' },
		{ value: 'anthropic',           label: 'Anthropic' },
		{ value: 'gemini',              label: 'Gemini' }
	];

	let models      = $state<Model[]>([]);
	let loading     = $state(false);
	let loadError   = $state('');

	type Mode = 'none' | 'create' | 'edit';
	let mode        = $state<Mode>('none');
	let editingId   = $state<string | null>(null);

	let fName       = $state('');
	let fProvider   = $state<Provider>('openai');
	let fBaseUrl    = $state('');
	let fModel      = $state('');
	let fApiKey     = $state('');
	let fParams     = $state('{}');
	let fEnabled    = $state(true);
	let fError      = $state('');
	let saving      = $state(false);

	let lastOpen = false;
	$effect(() => {
		if (open && !lastOpen) {
			void refresh();
		} else if (!open && lastOpen) {
			resetForm();
			mode = 'none';
		}
		lastOpen = open;
	});

	async function refresh() {
		loading = true;
		loadError = '';
		try {
			models = await listModels();
		} catch (e) {
			loadError = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	function resetForm() {
		fName = '';
		fProvider = 'openai';
		fBaseUrl = '';
		fModel = '';
		fApiKey = '';
		fParams = '{}';
		fEnabled = true;
		fError = '';
		editingId = null;
	}

	function beginCreate() {
		resetForm();
		mode = 'create';
	}

	function beginEdit(m: Model) {
		resetForm();
		editingId = m.id;
		fName = m.name;
		fProvider = m.provider;
		fBaseUrl = m.base_url ?? '';
		fModel = m.model;
		fApiKey = '';
		fParams = JSON.stringify(m.default_params ?? {}, null, 2);
		fEnabled = m.enabled;
		mode = 'edit';
	}

	function cancelForm() {
		resetForm();
		mode = 'none';
	}

	async function save() {
		fError = '';

		if (!fName.trim()) { fError = 'Name is required'; return; }
		if (!fModel.trim()) { fError = 'Model ID is required'; return; }
		if (mode === 'create' && !fApiKey.trim()) {
			fError = 'API key is required';
			return;
		}

		let parsedParams: Record<string, unknown> = {};
		const t = fParams.trim();
		if (t) {
			try {
				const p = JSON.parse(t);
				if (p && typeof p === 'object' && !Array.isArray(p)) {
					parsedParams = p as Record<string, unknown>;
				} else {
					fError = 'Default params must be a JSON object';
					return;
				}
			} catch {
				fError = 'Default params is not valid JSON';
				return;
			}
		}

		saving = true;
		try {
			const payload: CreateModelRequest = {
				name: fName.trim(),
				provider: fProvider,
				base_url: fBaseUrl.trim() || undefined,
				model: fModel.trim(),
				api_key: fApiKey,
				default_params: parsedParams,
				enabled: fEnabled
			};
			if (mode === 'create') {
				await createModel(payload);
			} else if (mode === 'edit' && editingId) {
				await updateModel(editingId, payload);
			}
			await refresh();
			cancelForm();
		} catch (e) {
			fError = e instanceof Error ? e.message : String(e);
		} finally {
			saving = false;
		}
	}

	async function removeModel(m: Model) {
		if (!confirm(`Delete model "${m.name}"? This cannot be undone.`)) return;
		try {
			await deleteModel(m.id);
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
			style="background:{cardBg}; border:1px solid {borderColor}; width:min(720px, 92vw); max-height:88vh;"
		>
			<div
				class="flex items-center justify-between px-6 py-4"
				style="border-bottom:1px solid {borderColor};"
			>
				<h2 style="font-size:16px; font-weight:600; color:{textPrimary};">Model Registry</h2>
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

				{#if mode === 'none'}
					<div class="flex items-center justify-between mb-3">
						<div style="font-size:13px; color:{textSecondary};">
							{models.length} model{models.length === 1 ? '' : 's'}
						</div>
						<button
							onclick={beginCreate}
							class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 cursor-pointer"
							style="background:{accent}; color:white; font-size:13px; font-weight:600; border:none;"
						>
							<PlusIcon class="w-3.5 h-3.5" />
							Add model
						</button>
					</div>

					{#if loading}
						<div style="color:{textMuted}; font-size:13px;">Loading…</div>
					{:else if models.length === 0}
						<div
							class="rounded-lg px-4 py-6 text-center"
							style="background:{surface2}; color:{textMuted}; font-size:13px; border:1px dashed {borderColor};"
						>
							No models yet. Add one to start optimizing prompts.
						</div>
					{:else}
						<div class="space-y-2">
							{#each models as m (m.id)}
								<div
									class="rounded-lg px-4 py-3 flex items-center justify-between gap-3"
									style="background:{surface2}; border:1px solid {borderColor};"
								>
									<div class="flex-1 min-w-0">
										<div class="flex items-center gap-2">
											<span style="font-size:14px; font-weight:600; color:{textPrimary};">{m.name}</span>
											<span
												class="rounded-full px-2"
												style="background:{accentTint}; color:{accent}; font-size:11px; font-weight:600;"
											>
												{m.provider}
											</span>
											{#if !m.enabled}
												<span
													class="rounded-full px-2"
													style="background:{borderColor}; color:{textMuted}; font-size:11px; font-weight:600;"
												>
													disabled
												</span>
											{/if}
										</div>
										<div style="font-size:12px; color:{textSecondary}; margin-top:2px;" class="truncate">
											{m.model}{m.base_url ? ' @ ' + m.base_url : ''}
										</div>
									</div>
									<button
										onclick={() => beginEdit(m)}
										aria-label="Edit"
										class="rounded-lg p-1.5 cursor-pointer"
										style="background:transparent; color:{textSecondary}; border:1px solid {borderColor};"
									>
										<PencilIcon class="w-3.5 h-3.5" />
									</button>
									<button
										onclick={() => removeModel(m)}
										aria-label="Delete"
										class="rounded-lg p-1.5 cursor-pointer"
										style="background:transparent; color:{danger}; border:1px solid {borderColor};"
									>
										<Trash2Icon class="w-3.5 h-3.5" />
									</button>
								</div>
							{/each}
						</div>
					{/if}
				{:else}
					<div class="space-y-3">
						<div>
							<label for="mm-name" style="font-size:12px; color:{textSecondary}; font-weight:500;">Name</label>
							<input
								id="mm-name"
								type="text"
								bind:value={fName}
								placeholder="e.g. OpenAI GPT-4o"
								class="w-full rounded-lg px-3 py-2 mt-1"
								style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
							/>
						</div>

						<div class="grid grid-cols-2 gap-3">
							<div>
								<label for="mm-provider" style="font-size:12px; color:{textSecondary}; font-weight:500;">Provider</label>
								<select
									id="mm-provider"
									bind:value={fProvider}
									class="w-full rounded-lg px-3 py-2 mt-1"
									style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
								>
									{#each providerOptions as p (p.value)}
										<option value={p.value}>{p.label}</option>
									{/each}
								</select>
							</div>
							<div>
								<label for="mm-model" style="font-size:12px; color:{textSecondary}; font-weight:500;">Model ID</label>
								<input
									id="mm-model"
									type="text"
									bind:value={fModel}
									placeholder="gpt-4o, claude-opus-4-7, gemini-2.5-pro…"
									class="w-full rounded-lg px-3 py-2 mt-1"
									style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
								/>
							</div>
						</div>

						<div>
							<label for="mm-baseurl" style="font-size:12px; color:{textSecondary}; font-weight:500;">
								Base URL <span style="color:{textMuted};">(optional)</span>
							</label>
							<input
								id="mm-baseurl"
								type="text"
								bind:value={fBaseUrl}
								placeholder="https://api.openai.com/v1"
								class="w-full rounded-lg px-3 py-2 mt-1"
								style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
							/>
						</div>

						<div>
							<label for="mm-apikey" style="font-size:12px; color:{textSecondary}; font-weight:500;">
								API Key {#if mode === 'edit'}<span style="color:{textMuted};">(leave blank to keep existing)</span>{/if}
							</label>
							<input
								id="mm-apikey"
								type="password"
								bind:value={fApiKey}
								placeholder={mode === 'edit' ? '•••••••• (unchanged)' : 'sk-…'}
								autocomplete="off"
								class="w-full rounded-lg px-3 py-2 mt-1"
								style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
							/>
						</div>

						<div>
							<label for="mm-params" style="font-size:12px; color:{textSecondary}; font-weight:500;">
								Default params <span style="color:{textMuted};">(JSON object)</span>
							</label>
							<textarea
								id="mm-params"
								bind:value={fParams}
								rows="4"
								placeholder={'{ "temperature": 0.7, "max_tokens": 2048 }'}
								class="w-full rounded-lg px-3 py-2 mt-1 font-mono"
								style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:12px;"
							></textarea>
						</div>

						<label class="flex items-center gap-2 cursor-pointer">
							<input type="checkbox" bind:checked={fEnabled} />
							<span style="font-size:13px; color:{textPrimary};">Enabled</span>
						</label>

						{#if fError}
							<div
								class="rounded-lg px-3 py-2"
								style="background:{dangerTint}; color:{danger}; font-size:13px;"
							>
								{fError}
							</div>
						{/if}
					</div>
				{/if}
			</div>

			<div
				class="flex items-center justify-end gap-2 px-6 py-3"
				style="border-top:1px solid {borderColor};"
			>
				{#if mode === 'none'}
					<button
						onclick={onClose}
						class="rounded-lg px-4 py-2 cursor-pointer"
						style="background:transparent; color:{textSecondary}; border:1px solid {borderColor}; font-size:13px;"
					>
						Close
					</button>
				{:else}
					<button
						onclick={cancelForm}
						disabled={saving}
						class="rounded-lg px-4 py-2 cursor-pointer"
						style="background:transparent; color:{textSecondary}; border:1px solid {borderColor}; font-size:13px;"
					>
						Cancel
					</button>
					<button
						onclick={save}
						disabled={saving}
						class="rounded-lg px-4 py-2 cursor-pointer"
						style="background:{accent}; color:white; border:none; font-size:13px; font-weight:600; opacity:{saving ? 0.6 : 1};"
					>
						{saving ? 'Saving…' : mode === 'create' ? 'Create' : 'Save'}
					</button>
				{/if}
			</div>
		</div>
	</div>
{/if}

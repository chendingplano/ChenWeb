<script lang="ts">
	import { onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import {
		checkPiGateway,
		getPiHarnessAdminState,
		initializePiHarnessProfiles,
		updatePiProfile,
		type PiHarnessAdminState,
		type PiProfile,
		type PiProfileCandidate
	} from '$lib/services/piHarnessAdminService';

	let { darkMode = true }: { darkMode?: boolean } = $props();
	let card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let text = $derived(darkMode ? '#E2E8F0' : '#111827');
	let muted = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let accent = $derived(darkMode ? '#A5B4FC' : '#4F46E5');
	let adminState = $state<PiHarnessAdminState | null>(null);
	let selectedSlug = $state('');
	let draft = $state<PiProfile | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let checking = $state(false);
	let initializing = $state(false);
	let error = $state('');
	let notice = $state('');
	let gatewayCheck = $state<{ status: 'healthy' | 'unavailable'; elapsed_ms: number; checked_at: string } | null>(null);
	let storesText = $state('');
	let groupsText = $state('');
	let pilotUsersText = $state('');
	let elapsedSeconds = $state(90);
	function requestStatus(error: unknown): number | undefined {
		return typeof error === 'object' && error !== null && 'status' in error && typeof error.status === 'number'
			? error.status
			: undefined;
	}

	function loadDraft(profile: PiProfile) {
		draft = structuredClone(profile);
		storesText = profile.allowed_knowledge_stores.join(', ');
		groupsText = profile.allowed_document_groups.join(', ');
		pilotUsersText = profile.pilot_users.join(', ');
		elapsedSeconds = profile.limits.max_elapsed;
	}

	async function load() {
		loading = true;
		error = '';
		try {
			adminState = await getPiHarnessAdminState();
			const preferred = adminState.profiles.find((p) => p.slug === selectedSlug) ?? adminState.profiles[0];
			if (preferred) {
				selectedSlug = preferred.slug;
				loadDraft(preferred);
			}
		} catch (e) {
			error = requestStatus(e) === 503 ? m.pi_harness_storage_unavailable() : m.pi_harness_load_failed();
		} finally {
			loading = false;
		}
	}

	onMount(() => void load());

	function selectProfile(slug: string) {
		selectedSlug = slug;
		const profile = adminState?.profiles.find((item) => item.slug === slug);
		if (profile) loadDraft(profile);
		error = '';
		notice = '';
	}

	function csv(value: string) {
		return value.split(',').map((part) => part.trim()).filter(Boolean);
	}

	async function save() {
		if (!draft || !adminState?.initialized) return;
		saving = true;
		error = '';
		notice = '';
		const candidate: PiProfileCandidate = {
			slug: draft.slug,
			friendly_name: draft.friendly_name,
			description: draft.description,
			provider: draft.provider,
			model: draft.model,
			thinking_level: draft.thinking_level,
			prompt_file: draft.prompt_file,
			allowed_knowledge_stores: csv(storesText),
			allowed_document_groups: csv(groupsText),
			permission_default: draft.permission_default,
			limits: { ...draft.limits, max_elapsed: elapsedSeconds },
			enabled: draft.enabled,
			pilot_users: csv(pilotUsersText),
			expected_active_version: draft.version
		};
		try {
			const result = await updatePiProfile(draft.slug, candidate);
			adminState = { ...adminState, profiles: adminState.profiles.map((p) => p.slug === result.profile.slug ? result.profile : p) };
			loadDraft(result.profile);
			notice = m.pi_harness_profile_saved({ version: result.profile.version });
		} catch (e) {
			if (requestStatus(e) === 409) {
				await load();
				error = m.pi_harness_version_conflict();
			} else {
				error = requestStatus(e) === 400 ? m.pi_harness_profile_invalid() : requestStatus(e) === 503 ? m.pi_harness_storage_unavailable() : m.pi_harness_save_failed();
			}
		} finally {
			saving = false;
		}
	}

	async function checkGateway() {
		checking = true;
		gatewayCheck = null;
		try {
			gatewayCheck = await checkPiGateway();
		} catch {
			gatewayCheck = { status: 'unavailable', elapsed_ms: 0, checked_at: new Date().toISOString() };
		}
		checking = false;
	}

	async function retryInitialization() {
		initializing = true;
		error = '';
		try {
			await initializePiHarnessProfiles();
			await load();
		} catch (e) {
			error = requestStatus(e) === 503 ? m.pi_harness_storage_unavailable() : m.pi_harness_initialize_failed();
		} finally {
			initializing = false;
		}
	}

	const inputClass = 'w-full rounded-md border px-3 py-2 text-sm outline-none focus:ring-2';
</script>

<div class="select-text h-full overflow-y-auto p-5 md:p-7" style="color:{text};">
	<div class="mx-auto flex max-w-6xl flex-col gap-5">
		<header>
			<div class="text-xs font-semibold uppercase tracking-[0.16em]" style="color:{accent};">{m.pi_harness_kicker()}</div>
			<h1 class="mt-1 text-2xl font-semibold">{m.pi_harness_title()}</h1>
			<p class="mt-2 max-w-3xl text-sm leading-6" style="color:{muted};">{m.pi_harness_intro()}</p>
		</header>

		{#if error}<div role="alert" class="rounded-lg border border-rose-500/40 bg-rose-500/10 px-4 py-3 text-sm">{error}</div>{/if}
		{#if notice}<div role="status" class="rounded-lg border border-emerald-500/40 bg-emerald-500/10 px-4 py-3 text-sm">{notice}</div>{/if}

		<section class="rounded-xl border p-5" style="background:{card}; border-color:{border};">
			<div class="flex flex-wrap items-start justify-between gap-4">
				<div>
					<h2 class="font-semibold">{m.pi_harness_gateway_heading()}</h2>
					<p class="mt-1 text-sm" style="color:{muted};">{m.pi_harness_gateway_help()}</p>
					<p class="mt-3 font-mono text-sm">{adminState?.gateway_url || m.pi_harness_gateway_unconfigured()}</p>
				</div>
				<button class="rounded-md px-4 py-2 text-sm font-medium" style="background:{accent}; color:#111827;" disabled={checking} onclick={checkGateway}>
					{checking ? m.pi_harness_checking() : m.pi_harness_check_connection()}
				</button>
			</div>
			<p class="mt-3 text-xs leading-5" style="color:{muted};">{m.pi_harness_deployment_managed()}</p>
			{#if gatewayCheck}
				<div class="mt-4 flex flex-wrap items-center gap-3 text-sm" role="status">
					<span class={gatewayCheck.status === 'healthy' ? 'text-emerald-500' : 'text-rose-500'}>{gatewayCheck.status === 'healthy' ? m.pi_harness_gateway_healthy() : m.pi_harness_gateway_unavailable()}</span>
					<span style="color:{muted};">{m.pi_harness_gateway_elapsed({ ms: gatewayCheck.elapsed_ms })}</span>
					<time style="color:{muted};">{new Intl.DateTimeFormat(getLocale(), { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(gatewayCheck.checked_at))}</time>
				</div>
			{/if}
		</section>

		{#if loading}
			<div class="rounded-xl border p-6 text-sm" style="background:{card}; border-color:{border}; color:{muted};">{m.pi_harness_loading()}</div>
		{:else if adminState && !adminState.initialized}
			<section class="rounded-xl border p-5" style="background:{card}; border-color:{border};">
				<h2 class="font-semibold">{m.pi_harness_initialization_required()}</h2>
				<p class="mt-2 text-sm" style="color:{muted};">{m.pi_harness_initialization_help()}</p>
				<button class="mt-4 rounded-md px-4 py-2 text-sm font-medium" style="background:{accent}; color:#111827;" disabled={initializing} onclick={retryInitialization}>
					{initializing ? m.pi_harness_initializing() : m.pi_harness_retry_initialization()}
				</button>
			</section>
		{:else if draft && adminState}
			<section class="rounded-xl border p-5" style="background:{card}; border-color:{border};">
				<div class="flex flex-wrap items-end justify-between gap-4 border-b pb-4" style="border-color:{border};">
					<div>
						<h2 class="font-semibold">{m.pi_harness_profile_heading()}</h2>
						<p class="mt-1 text-xs" style="color:{muted};">{m.pi_harness_active_version({ version: draft.version })}</p>
					</div>
					<label class="flex flex-col gap-1 text-sm"><span style="color:{muted};">{m.pi_harness_service()}</span>
						<select class={inputClass} style="background:{card}; border-color:{border}; color:{text};" value={selectedSlug} onchange={(e) => selectProfile(e.currentTarget.value)}>
							{#each adminState.profiles as profile (profile.slug)}<option value={profile.slug}>{profile.friendly_name}</option>{/each}
						</select>
					</label>
				</div>

				<p class="my-4 rounded-lg px-3 py-2 text-sm" style="background:{darkMode ? '#252A3A' : '#F3F4F6'}; color:{muted};">{m.pi_harness_provider_disclosure()}</p>
				<div class="grid gap-4 md:grid-cols-2">
					<label class="flex flex-col gap-1 text-sm"><span>{m.pi_harness_provider()}</span><input class={inputClass} style="background:{card}; border-color:{border}; color:{text};" bind:value={draft.provider} /></label>
					<label class="flex flex-col gap-1 text-sm"><span>{m.pi_harness_model()}</span><input class={inputClass} style="background:{card}; border-color:{border}; color:{text};" bind:value={draft.model} /></label>
					<label class="flex flex-col gap-1 text-sm"><span>{m.pi_harness_thinking()}</span><select class={inputClass} style="background:{card}; border-color:{border}; color:{text};" bind:value={draft.thinking_level}><option value="off">{m.pi_harness_thinking_off()}</option><option value="minimal">{m.pi_harness_thinking_minimal()}</option><option value="low">{m.pi_harness_thinking_low()}</option><option value="medium">{m.pi_harness_thinking_medium()}</option><option value="high">{m.pi_harness_thinking_high()}</option><option value="xhigh">{m.pi_harness_thinking_xhigh()}</option></select></label>
					<label class="flex flex-col gap-1 text-sm"><span>{m.pi_harness_prompt()}</span><select class={inputClass} style="background:{card}; border-color:{border}; color:{text};" bind:value={draft.prompt_file}>{#each adminState.prompt_choices[draft.slug] ?? [] as prompt}<option value={prompt}>{prompt}</option>{/each}</select></label>
					<label class="flex flex-col gap-1 text-sm"><span>{m.pi_harness_knowledge_stores()}</span><input class={inputClass} style="background:{card}; border-color:{border}; color:{text};" bind:value={storesText} /></label>
					<label class="flex flex-col gap-1 text-sm"><span>{m.pi_harness_document_groups()}</span><input class={inputClass} style="background:{card}; border-color:{border}; color:{text};" bind:value={groupsText} /></label>
					<label class="flex flex-col gap-1 text-sm"><span>{m.pi_harness_permission()}</span><select class={inputClass} style="background:{card}; border-color:{border}; color:{text};" bind:value={draft.permission_default}><option value="ask">{m.pi_harness_permission_ask()}</option><option value="auto">{m.pi_harness_permission_auto()}</option></select></label>
					<label class="flex flex-col gap-1 text-sm"><span>{m.pi_harness_pilot_users()}</span><input class={inputClass} style="background:{card}; border-color:{border}; color:{text};" bind:value={pilotUsersText} /></label>
					<label class="flex flex-col gap-1 text-sm"><span>{m.pi_harness_max_tool_calls()}</span><input class={inputClass} type="number" min="1" max="64" style="background:{card}; border-color:{border}; color:{text};" bind:value={draft.limits.max_tool_calls} /></label>
					<label class="flex flex-col gap-1 text-sm"><span>{m.pi_harness_max_elapsed()}</span><input class={inputClass} type="number" min="1" max="600" style="background:{card}; border-color:{border}; color:{text};" bind:value={elapsedSeconds} /></label>
					<label class="flex flex-col gap-1 text-sm"><span>{m.pi_harness_max_output_tokens()}</span><input class={inputClass} type="number" min="1" max="8192" style="background:{card}; border-color:{border}; color:{text};" bind:value={draft.limits.max_output_tokens} /></label>
					<label class="flex flex-col gap-1 text-sm"><span>{m.pi_harness_max_evidence_bytes()}</span><input class={inputClass} type="number" min="1" max="1048576" style="background:{card}; border-color:{border}; color:{text};" bind:value={draft.limits.max_evidence_bytes} /></label>
					<label class="flex flex-col gap-1 text-sm"><span>{m.pi_harness_max_history_tokens()}</span><input class={inputClass} type="number" min="1" max="128000" style="background:{card}; border-color:{border}; color:{text};" bind:value={draft.limits.max_history_tokens} /></label>
					<label class="flex flex-col gap-1 text-sm"><span>{m.pi_harness_max_tool_result_bytes()}</span><input class={inputClass} type="number" min="1" max="32768" style="background:{card}; border-color:{border}; color:{text};" bind:value={draft.limits.max_history_tool_result_bytes} /></label>
				</div>
				<label class="mt-4 flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={draft.enabled} />{m.pi_harness_enabled()}</label>
				<div class="mt-5">
					<div class="text-sm font-medium">{m.pi_harness_tools()}</div>
					<div class="mt-2 flex flex-wrap gap-2">{#each draft.allowed_tools as tool}<code class="rounded-md px-2 py-1 text-xs" style="background:{darkMode ? '#252A3A' : '#F3F4F6'};">{tool}</code>{/each}</div>
					<p class="mt-1 text-xs" style="color:{muted};">{m.pi_harness_tools_managed()}</p>
				</div>
				<div class="mt-6 flex justify-end"><button class="rounded-md px-4 py-2 text-sm font-semibold" style="background:{accent}; color:#111827;" disabled={saving} onclick={save}>{saving ? m.pi_harness_saving() : m.pi_harness_save_profile()}</button></div>
			</section>
		{/if}
	</div>
</div>

<script lang="ts">
	import { onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { AlertTriangle, Layers } from '@lucide/svelte';
	import {
		listProfiles,
		rerunReview,
		startProductReviewIntake,
		type Profile,
		type ProfileSummary
	} from '$lib/services/productMetricReviewService';

	// `embedded`: rendered inside content-panel.svelte's app shell, which already
	// supplies the breadcrumb/topbar — hide our own so it isn't shown twice.
	let { darkMode = false, embedded = false }: { darkMode?: boolean; embedded?: boolean } =
		$props();

	// ── form state ────────────────────────────────────────────────────────────
	let name = $state('');
	let description = $state('');
	let keywordsInput = $state('');
	let notes = $state('');
	let submitting = $state(false);
	let rerunning = $state(false);
	let error = $state('');

	// ── duplicate-choice state (spec: product-review-intake — a matching
	// product name surfaces a choice instead of a second profile) ─────────────
	let duplicateProfile = $state<Profile | null>(null);
	let duplicateLatestRequestId = $state<number | undefined>(undefined);
	let duplicateLatestRunId = $state<number | undefined>(undefined);

	// ── past-reviews list (spec: product-review-history-list) — selecting a
	// card prefills the form above and relabels the submit action "Re-Run",
	// independent of the duplicate-name hero above ─────────────────────────────
	let profiles = $state<ProfileSummary[]>([]);
	let loadingProfiles = $state(true);
	let selectedProfile = $state<ProfileSummary | null>(null);

	onMount(() => {
		loadProfiles();
	});

	async function loadProfiles() {
		loadingProfiles = true;
		try {
			const out = await listProfiles();
			profiles = out.profiles;
		} catch {
			// Past-reviews list is a convenience, not required to use the form —
			// leave it empty rather than surfacing a load error here.
		} finally {
			loadingProfiles = false;
		}
	}

	function selectProfile(p: ProfileSummary) {
		selectedProfile = p;
		name = p.name;
		description = p.product_description ?? '';
		keywordsInput = (p.keywords ?? []).join(', ');
		notes = p.notes ?? '';
		error = '';
		duplicateProfile = null;
	}

	// Editing the name away from the selected card drops the selection, so
	// "Re-Run" can never fire against a profile the visible name no longer
	// matches (design.md Decision 4).
	$effect(() => {
		if (selectedProfile && name.trim() !== selectedProfile.name.trim()) {
			selectedProfile = null;
		}
	});

	function statusWord(p: ProfileSummary): string {
		if (!p.latest_request_id) return m.pmr_intake_history_status_never_run();
		switch (p.latest_run_status) {
			case 'completed':
				return m.pmr_intake_history_status_completed();
			case 'running':
				return m.pmr_intake_history_status_running();
			case 'failed':
				return m.pmr_intake_history_status_failed();
			default:
				return m.pmr_intake_history_status_pending();
		}
	}

	function relativeTime(iso: string): string {
		const diffMs = Date.now() - new Date(iso).getTime();
		const mins = Math.round(diffMs / 60000);
		if (mins < 1) return 'just now';
		if (mins < 60) return `${mins}m ago`;
		const hours = Math.round(mins / 60);
		if (hours < 24) return `${hours}h ago`;
		const days = Math.round(hours / 24);
		if (days < 30) return `${days}d ago`;
		return `${Math.round(days / 30)}mo ago`;
	}

	const canStart = $derived(name.trim().length > 0 && !submitting);

	function parsedKeywords(): string[] {
		return keywordsInput
			.split(',')
			.map((k) => k.trim())
			.filter(Boolean);
	}

	function goToRun(runId: number) {
		if (typeof window === 'undefined') return;
		const u = new URL('/home3/product-metric-review', window.location.origin);
		u.searchParams.set('run', String(runId));
		u.searchParams.set('dark', darkMode ? '1' : '0');
		window.location.href = u.toString();
	}

	// Resumes an existing profile's review — re-running its latest request if
	// one exists, otherwise resume-building a profile that was created but
	// never run (design.md Decision 3; shared by the duplicate-name hero's
	// Re-run button and the past-reviews list's Re-Run button).
	async function resumeReview(
		profileId: number,
		profileName: string,
		requestId?: number,
		runId?: number
	): Promise<number> {
		if (requestId != null && runId != null) {
			const out = await rerunReview(requestId);
			return out.run.id;
		}
		const out = await startProductReviewIntake({
			name: profileName,
			resume_profile_id: profileId,
			notes: notes.trim()
		});
		if (!out.run) throw new Error('review did not start');
		return out.run.id;
	}

	async function start() {
		if (!canStart) return;
		submitting = true;
		error = '';
		duplicateProfile = null;
		try {
			if (selectedProfile) {
				const runId = await resumeReview(
					selectedProfile.id,
					selectedProfile.name,
					selectedProfile.latest_request_id,
					selectedProfile.latest_run_id
				);
				goToRun(runId);
				return;
			}
			const out = await startProductReviewIntake({
				name: name.trim(),
				product_description: description.trim(),
				keywords: parsedKeywords(),
				notes: notes.trim()
			});
			if (out.duplicate) {
				duplicateProfile = out.profile;
				duplicateLatestRequestId = out.latest_request_id;
				duplicateLatestRunId = out.latest_run?.id;
				return;
			}
			if (!out.run) throw new Error('review did not start');
			goToRun(out.run.id);
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			submitting = false;
		}
	}

	function viewResults() {
		if (duplicateLatestRunId != null) goToRun(duplicateLatestRunId);
	}

	async function handleRerun() {
		if (!duplicateProfile) return;
		rerunning = true;
		error = '';
		try {
			const runId = await resumeReview(
				duplicateProfile.id,
				duplicateProfile.name,
				duplicateLatestRequestId,
				duplicateLatestRunId
			);
			goToRun(runId);
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			rerunning = false;
		}
	}

	function startOver() {
		duplicateProfile = null;
		duplicateLatestRequestId = undefined;
		duplicateLatestRunId = undefined;
		error = '';
	}
</script>

<div class="pmr-shell" class:dark={darkMode} class:embedded>
	{#if !embedded}
		<header class="topbar">
			<div class="brand">
				<span class="brand-mark">PR</span>
				<div>
					<p class="kicker">{m.pmr_intake_kicker()}</p>
					<p class="brand-name">{m.pmr_intake_title()}</p>
				</div>
			</div>
			<div class="crumbs">
				<span class="route-badge">/home3/product-review</span>
			</div>
		</header>
	{/if}

	<div class="content">
		{#if error}
			<div class="note error">{error}</div>
		{/if}

		{#if duplicateProfile}
			<div class="empty-hero">
				<AlertTriangle size={22} />
				<h1>{m.pmr_intake_duplicate_heading()}</h1>
				<p>{m.pmr_intake_duplicate_hint({ name: duplicateProfile.name })}</p>
				{#if duplicateLatestRunId == null}
					<p class="muted">{m.pmr_intake_no_run_hint()}</p>
				{/if}
				<div class="run-open">
					{#if duplicateLatestRunId != null}
						<button class="ghost" onclick={viewResults}>{m.pmr_intake_view_results()}</button>
					{/if}
					<button class="primary" onclick={handleRerun} disabled={rerunning}>
						{rerunning ? m.pmr_intake_rerunning() : m.pmr_intake_rerun()}
					</button>
				</div>
				<button class="linky start-over" onclick={startOver}>{m.pmr_intake_start_over()}</button>
			</div>
		{:else}
			<div class="empty-hero">
				<Layers size={22} />
				<h1>{m.pmr_intake_heading()}</h1>
				<p>{m.pmr_intake_hint()}</p>
				<form
					class="intake-form"
					onsubmit={(e) => {
						e.preventDefault();
						start();
					}}
				>
					<label>
						<span>{m.pmr_intake_name_label()}</span>
						<input
							type="text"
							bind:value={name}
							placeholder={m.pmr_intake_name_placeholder()}
							disabled={submitting}
						/>
					</label>
					<label>
						<span>{m.pmr_intake_description_label()}</span>
						<textarea
							rows="2"
							bind:value={description}
							placeholder={m.pmr_intake_description_placeholder()}
							disabled={submitting}
						></textarea>
					</label>
					<label>
						<span>{m.pmr_intake_keywords_label()}</span>
						<input
							type="text"
							bind:value={keywordsInput}
							placeholder={m.pmr_intake_keywords_hint()}
							disabled={submitting}
						/>
					</label>
					<label>
						<span>{m.pmr_intake_notes_label()}</span>
						<textarea
							rows="2"
							bind:value={notes}
							placeholder={m.pmr_intake_notes_placeholder()}
							disabled={submitting}
						></textarea>
					</label>
					<button class="primary wide" type="submit" disabled={!canStart}>
						{#if selectedProfile}
							{submitting ? m.pmr_intake_rerunning() : m.pmr_intake_rerun()}
						{:else}
							{submitting ? m.pmr_intake_starting() : m.pmr_intake_start()}
						{/if}
					</button>
				</form>
			</div>
		{/if}

		<section class="history">
			<h2 class="history-heading">{m.pmr_intake_history_heading()}</h2>
			{#if loadingProfiles}
				<p class="muted">{m.pmr_intake_history_loading()}</p>
			{:else if profiles.length === 0}
				<p class="muted">{m.pmr_intake_history_empty()}</p>
			{:else}
				<div class="history-grid">
					{#each profiles as p (p.id)}
						<button
							type="button"
							class="history-card"
							class:selected={selectedProfile?.id === p.id}
							onclick={() => selectProfile(p)}
						>
							<div class="history-card-name">{p.name}</div>
							{#if p.product_description}
								<p class="history-card-desc">{p.product_description}</p>
							{/if}
							{#if p.keywords?.length}
								<div class="history-card-keywords">
									{#each p.keywords as kw (kw)}
										<span class="chip">{kw}</span>
									{/each}
								</div>
							{/if}
							<div class="history-card-status">
								{statusWord(p)}
								{#if p.latest_run_status === 'completed' && p.latest_run_finished_at}
									<span class="muted"> · {relativeTime(p.latest_run_finished_at)}</span>
								{/if}
							</div>
						</button>
					{/each}
				</div>
			{/if}
		</section>
	</div>
</div>

<style>
	@import url('https://fonts.googleapis.com/css2?family=DM+Mono:wght@400;500&family=Manrope:wght@400;500;600;700;800&display=swap');

	.pmr-shell {
		--bg: oklch(0.965 0.014 78);
		--surface: oklch(0.985 0.009 78);
		--text: oklch(0.23 0.025 63);
		--subtle: oklch(0.55 0.025 72);
		--border: oklch(0.88 0.028 75);
		--bronze: oklch(0.61 0.09 69);
		--red: oklch(0.57 0.14 27);
		--accent: #6366f1;
		min-height: 100vh;
		background: var(--bg);
		color: var(--text);
		font-family:
			'Manrope',
			-apple-system,
			BlinkMacSystemFont,
			'Segoe UI',
			sans-serif;
		transition:
			background 180ms ease,
			color 180ms ease;
	}
	.pmr-shell.embedded {
		min-height: 0;
	}
	.pmr-shell.dark {
		--bg: #111827;
		--surface: #182334;
		--text: #f7f5f1;
		--subtle: #c4cfdf;
		--border: #304663;
		--bronze: #ff9b54;
		--red: #ff7d6b;
		--accent: #818cf8;
	}
	.pmr-shell :global(*) {
		box-sizing: border-box;
	}
	.pmr-shell button,
	.pmr-shell input,
	.pmr-shell textarea {
		font: inherit;
		color: inherit;
	}
	.pmr-shell button:focus-visible,
	.pmr-shell input:focus-visible,
	.pmr-shell textarea:focus-visible {
		outline: 2px solid var(--bronze);
		outline-offset: 2px;
	}

	.kicker {
		margin: 0;
		text-transform: uppercase;
		letter-spacing: 0.13em;
		font:
			500 10px 'DM Mono',
			monospace;
		color: var(--bronze);
	}
	.muted {
		color: var(--subtle);
		font-size: 12px;
	}

	.topbar {
		height: 62px;
		padding: 0 clamp(16px, 3vw, 40px);
		border-bottom: 1px solid var(--border);
		display: flex;
		justify-content: space-between;
		align-items: center;
		background: color-mix(in oklch, var(--surface) 82%, transparent);
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 11px;
	}
	.brand-mark {
		width: 30px;
		height: 30px;
		border: 1px solid var(--bronze);
		color: var(--bronze);
		display: grid;
		place-items: center;
		font:
			500 11px 'DM Mono',
			monospace;
		letter-spacing: -0.06em;
	}
	.brand-name {
		margin: 2px 0 0;
		font-size: 13px;
		font-weight: 700;
	}
	.crumbs {
		display: flex;
		align-items: center;
		gap: 9px;
		color: var(--subtle);
		font-size: 12px;
	}
	.route-badge {
		padding: 3px 8px;
		border: 1px solid var(--border);
		border-radius: 99px;
		font:
			500 10px 'DM Mono',
			monospace;
	}

	.content {
		max-width: 720px;
		margin: 0 auto;
		padding: 26px clamp(16px, 2.4vw, 40px) 40px;
	}

	.note {
		display: flex;
		align-items: center;
		gap: 9px;
		margin: 12px 0;
		padding: 10px 12px;
		border: 1px solid var(--border);
		background: color-mix(in oklch, var(--surface) 60%, var(--bronze) 5%);
		font-size: 12px;
	}
	.note.error {
		color: var(--red);
		border-color: color-mix(in oklch, var(--red) 45%, var(--border));
	}

	.empty-hero {
		margin: 60px auto;
		max-width: 460px;
		text-align: center;
		color: var(--subtle);
	}
	.empty-hero h1 {
		margin: 14px 0 6px;
		font-size: 22px;
		letter-spacing: -0.03em;
		color: var(--text);
	}
	.empty-hero p {
		margin: 0 0 18px;
		font-size: 13px;
		line-height: 1.6;
	}

	.intake-form {
		display: flex;
		flex-direction: column;
		gap: 14px;
		text-align: left;
	}
	.intake-form label {
		display: flex;
		flex-direction: column;
		gap: 6px;
		font-size: 12px;
		color: var(--subtle);
	}
	.intake-form input,
	.intake-form textarea {
		padding: 9px 11px;
		border: 1px solid var(--border);
		background: var(--surface);
		color: var(--text);
		font-size: 13px;
		resize: vertical;
	}

	.run-open {
		display: flex;
		gap: 8px;
		justify-content: center;
		margin-top: 4px;
	}
	.ghost {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 9px 12px;
		border: 1px solid var(--border);
		background: transparent;
		color: var(--text);
		font-size: 12px;
		cursor: pointer;
	}
	.ghost:hover {
		background: var(--surface);
	}
	.primary {
		padding: 9px 14px;
		border: 1px solid var(--accent);
		background: var(--accent);
		color: #fff;
		cursor: pointer;
		font-size: 12px;
	}
	.primary:disabled {
		opacity: 0.6;
		cursor: wait;
	}
	.primary.wide {
		width: 100%;
		padding: 11px 14px;
		margin-top: 4px;
	}
	.linky {
		border: 0;
		background: transparent;
		color: var(--bronze);
		cursor: pointer;
		font-size: 11px;
		padding: 0;
	}
	.start-over {
		display: block;
		margin: 14px auto 0;
	}

	.history {
		margin-top: 32px;
		text-align: left;
	}
	.history-heading {
		margin: 0 0 12px;
		font-size: 13px;
		font-weight: 700;
		letter-spacing: -0.01em;
		color: var(--text);
	}
	.history-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
		gap: 12px;
	}
	.history-card {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 12px 14px;
		border: 1px solid var(--border);
		background: var(--surface);
		color: inherit;
		text-align: left;
		font: inherit;
		cursor: pointer;
	}
	.history-card:hover {
		border-color: var(--accent);
	}
	.history-card.selected {
		border-color: var(--accent);
		box-shadow: 0 0 0 1px var(--accent);
	}
	.history-card-name {
		font-size: 13px;
		font-weight: 600;
		color: var(--text);
	}
	.history-card-desc {
		margin: 0;
		font-size: 12px;
		line-height: 1.4;
		color: var(--subtle);
		display: -webkit-box;
		-webkit-line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.history-card-keywords {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}
	.chip {
		padding: 2px 7px;
		border: 1px solid var(--border);
		border-radius: 99px;
		font-size: 10px;
		color: var(--subtle);
	}
	.history-card-status {
		margin-top: 2px;
		font-size: 11px;
		color: var(--subtle);
	}
</style>

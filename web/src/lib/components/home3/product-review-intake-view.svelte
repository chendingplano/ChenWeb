<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { AlertTriangle, Layers } from '@lucide/svelte';
	import { rerunReview, startProductReviewIntake, type Profile } from '$lib/services/productMetricReviewService';

	let { darkMode = false }: { darkMode?: boolean } = $props();

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

	async function start() {
		if (!canStart) return;
		submitting = true;
		error = '';
		duplicateProfile = null;
		try {
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
			let runId: number;
			if (duplicateLatestRunId != null && duplicateLatestRequestId != null) {
				const out = await rerunReview(duplicateLatestRequestId);
				runId = out.run.id;
			} else {
				const out = await startProductReviewIntake({
					name: duplicateProfile.name,
					resume_profile_id: duplicateProfile.id,
					notes: notes.trim()
				});
				if (!out.run) throw new Error('review did not start');
				runId = out.run.id;
			}
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

<div class="pmr-shell" class:dark={darkMode}>
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
						{submitting ? m.pmr_intake_starting() : m.pmr_intake_start()}
					</button>
				</form>
			</div>
		{/if}
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
	.pmr-shell.dark {
		--bg: #111827;
		--surface: #182334;
		--text: #f7f5f1;
		--subtle: #c4cfdf;
		--border: #304663;
		--bronze: #ff9b54;
		--red: #ff7d6b;
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
		border: 1px solid var(--bronze);
		background: var(--bronze);
		color: var(--bg);
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
</style>

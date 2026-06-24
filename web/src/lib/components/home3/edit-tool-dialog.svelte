<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { getFindingLines, editFindingLines, type LineEdit } from '$lib/services/docReviewService';

	let {
		findingId,
		suggestion = '',
		dark = true,
		onsaved,
		oncancel
	}: {
		findingId: number;
		suggestion?: string;
		dark?: boolean;
		onsaved: (changed: number) => void;
		oncancel: () => void;
	} = $props();

	let loading = $state(true);
	let saving = $state(false);
	let errorMsg = $state('');
	let lines = $state<LineEdit[]>([]);

	let search = $state('');
	let replaceText = $state('');

	// Current match position within `lines`.
	let matchLine = $state<number | null>(null);
	let matchStart = $state(0);
	let matchEnd = $state(0);
	let noMatchNotice = $state(false);

	let inputs: HTMLTextAreaElement[] = [];

	let hasMatch = $derived(matchLine !== null);
	let canFind = $derived(search.length > 0 && lines.length > 0);

	// Many suggestions follow the pattern "<instruction>例如：'<proposed content>'"
	// (or an English "e.g."/quoted variant). Split off the quoted proposed content so
	// the reviewer can accept it directly. Returns null when no quoted content is found.
	function splitSuggestion(s: string): { intro: string; proposed: string } | null {
		if (!s) return null;
		const pairs: [string, string][] = [
			['‘', '’'], // ' '  curly single
			['“', '”'], // " "  curly double
			['「', '」'], // 「 」 corner
			['『', '』'], // 『 』 white corner
			['"', '"'],
			["'", "'"]
		];
		let best: { intro: string; proposed: string } | null = null;
		for (const [open, close] of pairs) {
			const start = s.indexOf(open);
			if (start === -1) continue;
			const end = s.lastIndexOf(close);
			if (end <= start + 1) continue;
			const proposed = s.slice(start + 1, end).trim();
			if (!proposed) continue;
			if (!best || proposed.length > best.proposed.length) {
				best = { intro: s.slice(0, start).trim(), proposed };
			}
		}
		return best;
	}

	let parsedSuggestion = $derived(splitSuggestion(suggestion));

	// Accept the parsed suggestion: replace the (first) offending line with the
	// proposed content. The reviewer still reviews/edits and clicks Save to persist.
	async function acceptSuggestion() {
		if (!parsedSuggestion || lines.length === 0) return;
		lines[0] = { ...lines[0], content: parsedSuggestion.proposed };
		resetMatch();
		await tick();
		if (inputs[0]) fitTextarea(inputs[0]);
	}

	onMount(async () => {
		try {
			lines = await getFindingLines(findingId);
			if (lines.length === 0) {
				errorMsg = 'No editable source line is linked to this finding.';
			}
		} catch (e) {
			errorMsg = e instanceof Error ? e.message : 'Failed to load lines';
		} finally {
			loading = false;
		}
	});

	// Grow a textarea to fit its content (so the whole offending line is visible),
	// plus a small bottom margin. min-height (CSS) keeps short lines reasonable.
	function fitTextarea(el: HTMLTextAreaElement) {
		el.style.height = 'auto';
		el.style.height = `${el.scrollHeight + 6}px`;
	}

	// Svelte action: size the offending-line textarea to its content on mount and
	// keep it fitted as the user types.
	function autosize(node: HTMLTextAreaElement) {
		const handler = () => fitTextarea(node);
		requestAnimationFrame(handler);
		node.addEventListener('input', handler);
		return {
			destroy() {
				node.removeEventListener('input', handler);
			}
		};
	}

	function resetMatch() {
		matchLine = null;
		noMatchNotice = false;
	}

	function selectMatch() {
		if (matchLine === null) return;
		const el = inputs[matchLine];
		if (el) {
			el.focus();
			try {
				el.setSelectionRange(matchStart, matchEnd);
			} catch {
				/* non-text input */
			}
		}
	}

	// Finds the next occurrence of `search`, wrapping around, continuing past the
	// current match. Updates match state and selects it; clears match if none.
	function findNext() {
		if (!search || lines.length === 0) return;
		const n = lines.length;
		let startLine = 0;
		let startOffset = 0;
		if (matchLine !== null) {
			startLine = matchLine;
			startOffset = matchEnd;
		}
		for (let k = 0; k < n; k++) {
			const li = (startLine + k) % n;
			const from = k === 0 ? startOffset : 0;
			const idx = lines[li].content.indexOf(search, from);
			if (idx !== -1) {
				matchLine = li;
				matchStart = idx;
				matchEnd = idx + search.length;
				noMatchNotice = false;
				selectMatch();
				return;
			}
		}
		matchLine = null;
		noMatchNotice = true;
	}

	function applyReplacement(removeOnly: boolean) {
		if (matchLine === null) return;
		const li = matchLine;
		const c = lines[li].content;
		const repl = removeOnly ? '' : replaceText;
		lines[li] = { ...lines[li], content: c.slice(0, matchStart) + repl + c.slice(matchEnd) };
		// Continue searching from just after the replacement text.
		matchStart = matchStart + repl.length;
		matchEnd = matchStart;
		findNext();
	}

	async function save() {
		saving = true;
		errorMsg = '';
		try {
			const changed = await editFindingLines(findingId, lines);
			onsaved(changed);
		} catch (e) {
			errorMsg = e instanceof Error ? e.message : 'Failed to save';
			saving = false;
		}
	}

	function onBackdrop(e: MouseEvent) {
		if (e.target === e.currentTarget) oncancel();
	}
</script>

<div
	class="backdrop"
	class:dark
	role="presentation"
	onclick={onBackdrop}
>
	<div class="dialog" role="dialog" aria-modal="true" aria-label="Edit Tool">
		<h2 class="title">Edit Tool</h2>

		{#if loading}
			<div class="state">Loading source line(s)…</div>
		{:else}
			{#if errorMsg}<div class="state error">{errorMsg}</div>{/if}

			{#if suggestion}
				<div class="section-label">Suggestion</div>
				{#if parsedSuggestion}
					{#if parsedSuggestion.intro}
						<div class="suggestion-intro">{parsedSuggestion.intro}</div>
					{/if}
					<div class="suggestion-proposed">
						<div class="suggestion-proposed-text">{parsedSuggestion.proposed}</div>
						<button
							class="accept-sugg"
							disabled={lines.length === 0}
							onclick={acceptSuggestion}
							title="Replace the offending line with this suggested content"
						>
							Accept
						</button>
					</div>
				{:else}
					<div class="suggestion">{suggestion}</div>
				{/if}
			{/if}

			<div class="section-label">Offending line(s)</div>
			<div class="lines">
				{#each lines as line, i (line.line_no)}
					<div class="line-row">
						<span class="line-no">L{line.line_no}</span>
						<textarea
							class="line-input"
							bind:this={inputs[i]}
							bind:value={lines[i].content}
							oninput={resetMatch}
							use:autosize
							spellcheck="false"
							rows="1"
						></textarea>
					</div>
				{/each}
				{#if lines.length === 0 && !errorMsg}
					<div class="state">No lines to edit.</div>
				{/if}
			</div>

			<div class="find-replace">
				<div class="fr-row">
					<label class="fr-label" for="et-search">Find</label>
					<input
						id="et-search"
						class="fr-input"
						placeholder="substring to search"
						bind:value={search}
						oninput={resetMatch}
						spellcheck="false"
					/>
				</div>
				<div class="fr-row">
					<label class="fr-label" for="et-replace">Replace</label>
					<input
						id="et-replace"
						class="fr-input"
						placeholder="replacement text"
						bind:value={replaceText}
						spellcheck="false"
					/>
				</div>
				<div class="fr-buttons">
					<button class="btn" disabled={!canFind} onclick={findNext}>Find</button>
					<button class="btn" disabled={!hasMatch} onclick={() => applyReplacement(false)}>Replace</button>
					<button class="btn" disabled={!hasMatch} onclick={() => applyReplacement(true)}>Remove</button>
				</div>
				{#if noMatchNotice}
					<div class="fr-note">No match found.</div>
				{:else if hasMatch}
					<div class="fr-note ok">Match on line L{lines[matchLine!].line_no}.</div>
				{/if}
			</div>
		{/if}

		<div class="actions">
			<button class="btn ghost" onclick={oncancel} disabled={saving}>Cancel</button>
			<button class="btn primary" onclick={save} disabled={saving || loading || lines.length === 0}>
				{saving ? 'Saving…' : 'Save'}
			</button>
		</div>
	</div>
</div>

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1000;
		padding: 1rem;
	}
	.dialog {
		width: min(680px, 100%);
		max-height: 90vh;
		overflow-y: auto;
		background: #ffffff;
		color: #111827;
		border-radius: 12px;
		padding: 1.25rem 1.5rem;
		box-shadow: 0 20px 60px rgba(0, 0, 0, 0.35);
		font-family: system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif;
	}
	.backdrop.dark .dialog {
		background: #1f2333;
		color: #e2e8f0;
		border: 1px solid #2d3348;
	}
	.title {
		margin: 0 0 1rem;
		font-size: 1.15rem;
	}
	.section-label {
		font-size: 0.72rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: #6b7280;
		margin: 0.5rem 0 0.35rem;
	}
	.suggestion {
		font-size: 0.86rem;
		line-height: 1.5;
		padding: 0.55rem 0.7rem;
		border-radius: 6px;
		background: rgba(99, 102, 241, 0.08);
		border: 1px solid rgba(99, 102, 241, 0.25);
		color: #374151;
		margin-bottom: 1rem;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.backdrop.dark .suggestion {
		background: rgba(129, 140, 248, 0.12);
		border-color: rgba(129, 140, 248, 0.3);
		color: #cbd5e1;
	}
	.suggestion-intro {
		font-size: 0.82rem;
		line-height: 1.5;
		color: #6b7280;
		margin-bottom: 0.45rem;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.backdrop.dark .suggestion-intro {
		color: #94a3b8;
	}
	.suggestion-proposed {
		display: flex;
		align-items: flex-start;
		gap: 0.6rem;
		padding: 0.55rem 0.7rem;
		border-radius: 6px;
		background: rgba(16, 185, 129, 0.08);
		border: 1px solid rgba(16, 185, 129, 0.3);
		margin-bottom: 1rem;
	}
	.backdrop.dark .suggestion-proposed {
		background: rgba(16, 185, 129, 0.12);
		border-color: rgba(16, 185, 129, 0.35);
	}
	.suggestion-proposed-text {
		flex: 1 1 auto;
		font-size: 0.86rem;
		line-height: 1.5;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		color: #374151;
	}
	.backdrop.dark .suggestion-proposed-text {
		color: #cbd5e1;
	}
	.accept-sugg {
		flex: 0 0 auto;
		align-self: center;
		padding: 0.4rem 0.85rem;
		border: 1px solid #10b981;
		border-radius: 6px;
		background: #10b981;
		color: #fff;
		font: inherit;
		font-size: 0.8rem;
		font-weight: 600;
		cursor: pointer;
		white-space: nowrap;
	}
	.accept-sugg:hover:not(:disabled) {
		background: #059669;
		border-color: #059669;
	}
	.accept-sugg:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.lines {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		margin-bottom: 1rem;
	}
	.line-row {
		display: flex;
		align-items: flex-start;
		gap: 0.5rem;
	}
	.line-no {
		flex: 0 0 auto;
		font-size: 0.72rem;
		color: #6b7280;
		min-width: 2.75rem;
		font-variant-numeric: tabular-nums;
		padding-top: 0.55rem;
	}
	.line-input,
	.fr-input {
		flex: 1 1 auto;
		width: 100%;
		padding: 0.5rem 0.6rem;
		border: 1px solid #d1d5db;
		border-radius: 6px;
		font: inherit;
		font-size: 0.88rem;
		background: #ffffff;
		color: #111827;
		box-sizing: border-box;
	}
	textarea.line-input {
		resize: vertical;
		min-height: 2.75rem;
		line-height: 1.45;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.backdrop.dark .line-input,
	.backdrop.dark .fr-input {
		background: #15181f;
		color: #e2e8f0;
		border-color: #2d3348;
	}
	.find-replace {
		border-top: 1px solid #e4e6eb;
		padding-top: 0.9rem;
	}
	.backdrop.dark .find-replace {
		border-top-color: #2d3348;
	}
	.fr-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-bottom: 0.5rem;
	}
	.fr-label {
		flex: 0 0 auto;
		min-width: 3.5rem;
		font-size: 0.8rem;
		color: #6b7280;
	}
	.fr-buttons {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.25rem;
	}
	.fr-note {
		margin-top: 0.5rem;
		font-size: 0.78rem;
		color: #dc2626;
	}
	.fr-note.ok {
		color: #10b981;
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 1.25rem;
	}
	.btn {
		padding: 0.45rem 0.9rem;
		border: 1px solid #d1d5db;
		border-radius: 6px;
		background: #f3f4f6;
		color: #111827;
		font: inherit;
		font-size: 0.84rem;
		cursor: pointer;
		transition: background 0.15s, opacity 0.15s;
	}
	.backdrop.dark .btn {
		background: #2d3348;
		color: #e2e8f0;
		border-color: #3a4158;
	}
	.btn:hover:not(:disabled) {
		background: #e5e7eb;
	}
	.backdrop.dark .btn:hover:not(:disabled) {
		background: #3a4158;
	}
	.btn:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}
	.btn.primary {
		background: #6366f1;
		border-color: #6366f1;
		color: #ffffff;
	}
	.btn.primary:hover:not(:disabled) {
		background: #4f46e5;
	}
	.btn.ghost {
		background: transparent;
	}
	.state {
		padding: 0.75rem 0;
		color: #6b7280;
		font-size: 0.88rem;
	}
	.state.error {
		color: #dc2626;
	}
</style>

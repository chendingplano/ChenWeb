<script lang="ts">
	import { onDestroy } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { movableDialog } from '$lib/components/shared-ui/movable-dialog';

	type Decision = 'cancel' | 'skip' | 'include';

	let { darkMode }: { darkMode: boolean } = $props();
	const titleID = $props.id();
	let dialog = $state<HTMLDialogElement>();
	let pending = $state<{
		anchor: DOMRect;
		total: number;
		remoteCount: number;
		resolve: (decision: Decision) => void;
	}>();

	export function ask(anchor: DOMRect, total: number, remoteCount: number) {
		return new Promise<Decision>((resolve) => {
			pending = { anchor, total, remoteCount, resolve };
		});
	}

	function decide(decision: Decision) {
		const resolve = pending?.resolve;
		dialog?.close();
		pending = undefined;
		resolve?.(decision);
	}

	$effect(() => {
		if (!dialog || !pending) return;
		dialog.showModal();
		dialog.querySelector<HTMLButtonElement>('.safe')?.focus();
	});

	onDestroy(() => decide('cancel'));

</script>

{#if pending}
{@const { anchor, total, remoteCount } = pending}
<dialog
	bind:this={dialog}
	class:dark={darkMode}
	aria-labelledby={titleID}
	oncancel={(event) => {
		event.preventDefault();
		decide('cancel');
	}}
>
	<header use:movableDialog={anchor}>
		<h2 id={titleID}>{m.doc_origin_confirmation_title()}</h2>
	</header>
	<div class="body">
		<p>{total === 1 ? m.doc_origin_confirmation_one() : m.doc_origin_confirmation_many({ count: remoteCount })}</p>
		<p>{m.doc_origin_confirmation_overwrite()}</p>
	</div>
	<footer>
		<button type="button" class:safe={total === 1} class:default={total === 1} onclick={() => decide('cancel')}>
			{m.doc_origin_confirmation_cancel()}
		</button>
		{#if total > 1}
			<button type="button" class="default safe" onclick={() => decide('skip')}>{m.doc_origin_confirmation_skip()}</button>
		{/if}
		<button type="button" onclick={() => decide('include')}>
			{total === 1 ? m.doc_origin_confirmation_run() : m.doc_origin_confirmation_run_all()}
		</button>
	</footer>
</dialog>
{/if}

<style>
	dialog {
		--surface: #fffaf0;
		--ink: #231c11;
		--line: #d3c5a9;
		--accent: #825419;
		position: fixed;
		box-sizing: border-box;
		width: min(420px, calc(100vw - 16px));
		min-width: min(264px, calc(100vw - 16px));
		min-height: min(150px, calc(100dvh - 16px));
		max-width: calc(100vw - 16px);
		max-height: calc(100dvh - 16px);
		padding: 0;
		overflow: auto;
		resize: both;
		user-select: text;
		background: var(--surface);
		color: var(--ink);
		border: 1px solid var(--line);
		border-radius: 10px;
		box-shadow: 0 16px 40px #0d080540;
	}
	dialog.dark {
		--surface: #1a202b;
		--ink: #f3eedf;
		--line: #626575;
		--accent: #f0bc6c;
	}
	dialog::backdrop { background: #0c101448; }
	header {
		padding: 12px 16px;
		border-bottom: 1px solid var(--line);
		cursor: move;
		touch-action: none;
		user-select: none;
	}
	h2 { font-size: 15px; font-weight: 650; margin: 0; }
	.body { padding: 12px 16px; font-size: 14px; line-height: 1.55; }
	p + p { margin-top: 8px; }
	footer { display: flex; flex-wrap: wrap; justify-content: end; gap: 6px; padding: 2px 16px 16px; }
	button { border: 1px solid var(--line); border-radius: 6px; padding: 6px 10px; font-size: 13px; }
	button:hover, button:focus-visible { border-color: var(--accent); }
	.default { border-color: var(--accent); color: var(--accent); font-weight: 650; }
</style>

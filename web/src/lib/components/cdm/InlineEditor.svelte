<script lang="ts">
	// Mounts a real ProseMirror-backed editor (via @tiptap/core's Editor) over
	// one block's inline content, using the schema and mapping from
	// inline-schema.ts / inline-mapping.ts. This is the "TipTap confined to
	// inline content" half of ADR 2026072603 DR1 -- one instance edits one
	// paragraph/heading/quote block's `content: Inline[]`, never a sequence
	// of blocks (BlockList already owns that).
	//
	// Editor creation/teardown uses onMount/onDestroy, not $effect: an effect
	// tracks every value it reads as a dependency, and this component both
	// reads and writes `content` (writes on every edit, via onUpdate below).
	// If editor creation ran inside $effect, writing to `content` would
	// re-trigger the same effect, destroying and recreating the editor on
	// every keystroke. onMount/onDestroy are plain lifecycle hooks with no
	// dependency tracking, which is what a one-time "mount a DOM library,
	// tear it down on unmount" job needs.
	//
	// `as`/`level` replace wrapping this component in a real <p>/<hN>/
	// <blockquote> element. ProseMirror's EditorView always creates its own
	// contenteditable root as a <div> regardless of the container passed to
	// it -- confirmed by inspecting the rendered DOM -- so nesting that
	// output inside a <p> produces invalid HTML (a block-level <div> is not
	// permitted inside <p>, which only accepts phrasing content) and a real
	// hydration-mismatch warning, not just a lint nitpick. The standard fix
	// for contenteditable-based editors (used by Notion, Google Docs, and
	// effectively every div-based rich editor) is styling the div to look
	// like the target element instead of nesting inside one; `role`/
	// `aria-level` restore the semantic signal a screen reader would
	// otherwise have gotten from a literal heading tag.
	//
	// The formatting toolbar (task 5.6) lives here, per instance, rather
	// than as one global toolbar shared across every editable block: that
	// would need a registry tracking which of potentially many InlineEditor
	// instances currently has focus, which nothing else in this change needs
	// yet. Scoped to this instance, shown only while it has focus. Offers
	// exactly strong/emphasis/code/link -- the four CDM inline types that
	// are marks -- and nothing else: no font, size, colour, or alignment
	// control (design D1). Block-level semantic actions (heading level,
	// list/table/quote/code-block/equation/image/callout) are BlockList's
	// type-change control and InsertControl (task group 4), not duplicated
	// here.
	import { onMount, onDestroy } from 'svelte';
	import { Editor } from '@tiptap/core';
	import { INLINE_EXTENSIONS, buildInlineSchema } from './inline-schema.js';
	import { inlineArrayToProseMirrorDoc, proseMirrorDocToInlineArray } from './inline-mapping.js';
	import type { Inline } from './types.js';

	let {
		content = $bindable(),
		as = 'paragraph',
		level = 2
	}: { content: Inline[]; as?: 'paragraph' | 'heading' | 'quote'; level?: number } = $props();

	let containerEl: HTMLDivElement;
	let editor: Editor | undefined;
	let focused = $state(false);
	// Bumped on every transaction so the $derived active-mark checks below
	// re-evaluate; editor.isActive() itself is a plain method call with no
	// reactivity of its own.
	let version = $state(0);

	onMount(() => {
		const schema = buildInlineSchema();
		const initialDoc = inlineArrayToProseMirrorDoc(schema, content);

		editor = new Editor({
			element: containerEl,
			extensions: INLINE_EXTENSIONS,
			content: initialDoc.toJSON(),
			onUpdate: ({ editor, transaction }) => {
				if (!transaction.docChanged) return;
				content = proseMirrorDocToInlineArray(editor.state.doc);
			},
			onTransaction: () => {
				version++;
			},
			onFocus: () => {
				focused = true;
			},
			onBlur: () => {
				focused = false;
			}
		});
	});

	onDestroy(() => {
		editor?.destroy();
	});

	let clampedLevel = $derived(Math.min(6, Math.max(1, level)));

	let isStrong = $derived.by(() => {
		void version;
		return editor?.isActive('strong') ?? false;
	});
	let isEmphasis = $derived.by(() => {
		void version;
		return editor?.isActive('emphasis') ?? false;
	});
	let isCode = $derived.by(() => {
		void version;
		return editor?.isActive('code') ?? false;
	});
	let isLink = $derived.by(() => {
		void version;
		return editor?.isActive('link') ?? false;
	});

	function toggle(mark: 'strong' | 'emphasis' | 'code') {
		editor?.chain().focus().toggleMark(mark).run();
	}

	function toggleLink() {
		if (!editor) return;
		if (isLink) {
			editor.chain().focus().unsetMark('link').run();
			return;
		}
		// A prompt is deliberate v1 scaffolding, not a design decision: this
		// task is the schema/mapping/toolbar plumbing, not a URL-input
		// dialog. Swapping it for a proper popover later does not touch
		// inline-schema.ts, inline-mapping.ts, or the command it calls here.
		const url = typeof window !== 'undefined' ? window.prompt('Link URL') : null;
		if (!url) return;
		editor.chain().focus().toggleMark('link', { url }).run();
	}
</script>

<div class="cdm-inline-editor-wrap">
	{#if focused}
		<div class="cdm-inline-toolbar">
			<button
				type="button"
				class:active={isStrong}
				onmousedown={(e) => e.preventDefault()}
				onclick={() => toggle('strong')}><strong>B</strong></button
			>
			<button
				type="button"
				class:active={isEmphasis}
				onmousedown={(e) => e.preventDefault()}
				onclick={() => toggle('emphasis')}><em>I</em></button
			>
			<button
				type="button"
				class:active={isCode}
				onmousedown={(e) => e.preventDefault()}
				onclick={() => toggle('code')}><code>{'<>'}</code></button
			>
			<button
				type="button"
				class:active={isLink}
				onmousedown={(e) => e.preventDefault()}
				onclick={toggleLink}>Link</button
			>
		</div>
	{/if}
	<div
		bind:this={containerEl}
		class="cdm-inline-editor"
		data-as={as}
		data-level={as === 'heading' ? clampedLevel : undefined}
		role={as === 'heading' ? 'heading' : undefined}
		aria-level={as === 'heading' ? clampedLevel : undefined}
	></div>
</div>

<style>
	.cdm-inline-toolbar {
		display: flex;
		gap: 2px;
		margin-bottom: 2px;
	}
	.cdm-inline-toolbar button {
		border: 1px solid rgba(127, 127, 127, 0.35);
		border-radius: 4px;
		background: transparent;
		padding: 1px 8px;
		font-size: 0.8em;
		line-height: 1.6;
		cursor: pointer;
	}
	.cdm-inline-toolbar button.active {
		background: rgba(37, 99, 235, 0.15);
		border-color: rgba(37, 99, 235, 0.6);
	}
	.cdm-inline-editor :global(.tiptap) {
		outline: none;
		min-height: 1.4em;
	}
	.cdm-inline-editor[data-as='heading'] :global(.tiptap) {
		font-weight: 700;
	}
	.cdm-inline-editor[data-as='heading'][data-level='1'] :global(.tiptap) {
		font-size: 1.6em;
	}
	.cdm-inline-editor[data-as='heading'][data-level='2'] :global(.tiptap) {
		font-size: 1.4em;
	}
	.cdm-inline-editor[data-as='heading'][data-level='3'] :global(.tiptap) {
		font-size: 1.2em;
	}
	.cdm-inline-editor[data-as='heading'][data-level='4'] :global(.tiptap) {
		font-size: 1.1em;
	}
	.cdm-inline-editor[data-as='heading'][data-level='5'] :global(.tiptap),
	.cdm-inline-editor[data-as='heading'][data-level='6'] :global(.tiptap) {
		font-size: 1em;
	}
	.cdm-inline-editor[data-as='quote'] :global(.tiptap) {
		border-left: 3px solid rgba(127, 127, 127, 0.35);
		padding: 2px 0 2px 12px;
		font-style: italic;
	}
</style>

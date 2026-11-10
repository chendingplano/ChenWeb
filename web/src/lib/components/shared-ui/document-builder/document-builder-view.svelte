<script lang="ts">
	// Document Builder viewer: a category tree on the left, the selected document on the right.
	// Markdown goes through the Markdown Viewer module (20261109-01); PDFs use the browser's own
	// viewer in an iframe. See KnowledgeStore/doc-repo/modules/module-document-builder-frontend.md.
	import { onMount } from 'svelte';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import FolderIcon from '@lucide/svelte/icons/folder';
	import FolderOpenIcon from '@lucide/svelte/icons/folder-open';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import FileIcon from '@lucide/svelte/icons/file';
	import { m } from '$lib/paraglide/messages.js';
	import MarkdownViewer from '$lib/components/shared-ui/markdown-viewer/markdown-viewer.svelte';
	import {
		docGroupFileUrl,
		docGroupPdfUrl,
		docGroupTreeUrl,
		findLeaf,
		firstLeaf,
		stripFrontMatter,
		type DocTreeNode,
		type DocTreeResponse
	} from './document-builder-types';

	let {
		group,
		darkMode = true,
		leftWidthDefault = 260
	}: { group: string; darkMode?: boolean; leftWidthDefault?: number } = $props();

	const LEFT_MIN = 180;
	const LEFT_MAX = 480;

	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let panelBg = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textMuted = $derived(darkMode ? '#94A3B8' : '#6B7280');

	let tree = $state<DocTreeNode[]>([]);
	let pending = $state(0);
	let expanded = $state<Record<string, boolean>>({});
	let selected = $state<DocTreeNode | null>(null);
	let markdown = $state('');
	let loading = $state(true);
	let error = $state<string | null>(null);
	let leftWidth = $state(leftWidthDefault);
	let contentEl = $state<HTMLElement | null>(null);
	let scrollTops: Record<number, number> = {};

	// Work is kept per group for the browser session so leaving and returning restores it.
	const storeKey = () => `docbuilder:${group}`;
	function saveState() {
		try {
			sessionStorage.setItem(
				storeKey(),
				JSON.stringify({ docId: selected?.doc_id, expanded, leftWidth, scrollTops })
			);
		} catch {
			/* storage unavailable: nothing to restore next time */
		}
	}
	function restoreState(): { docId?: number } {
		try {
			const s = JSON.parse(sessionStorage.getItem(storeKey()) ?? '{}');
			expanded = s.expanded ?? {};
			leftWidth = s.leftWidth ?? leftWidthDefault;
			scrollTops = s.scrollTops ?? {};
			return { docId: s.docId };
		} catch {
			return {};
		}
	}

	async function select(node: DocTreeNode) {
		if (!node.doc_id) return;
		if (selected?.doc_id && contentEl) scrollTops[selected.doc_id] = contentEl.scrollTop;
		selected = node;
		error = null;
		markdown = '';
		if (node.kind === 'markdown') {
			try {
				const r = await fetch(docGroupFileUrl(group, node.doc_id), { credentials: 'same-origin' });
				if (!r.ok) throw new Error(String(r.status));
				markdown = stripFrontMatter(await r.text());
			} catch {
				error = m.docbuilder_load_failed({ label: node.label });
			}
		}
		saveState();
		requestAnimationFrame(() => {
			if (contentEl) contentEl.scrollTop = scrollTops[node.doc_id!] ?? 0;
		});
	}

	onMount(async () => {
		const saved = restoreState();
		try {
			const r = await fetch(docGroupTreeUrl(group), { credentials: 'same-origin' });
			if (!r.ok) throw new Error(String(r.status));
			const data = (await r.json()) as DocTreeResponse;
			tree = data.tree;
			pending = data.uncategorized;
			const restored = saved.docId ? findLeaf(tree, saved.docId) : undefined;
			const start = restored ?? firstLeaf(tree);
			if (start) {
				for (const c of tree)
					if (c.children?.some((l) => l.doc_id === start.doc_id)) expanded[c.id] = true;
				await select(start);
			}
		} catch {
			error = m.docbuilder_tree_failed();
		} finally {
			loading = false;
		}
	});

	let dragStartX = 0;
	let dragStartWidth = 0;
	function startDrag(e: MouseEvent) {
		dragStartX = e.clientX;
		dragStartWidth = leftWidth;
		const move = (ev: MouseEvent) => {
			leftWidth = Math.max(LEFT_MIN, Math.min(LEFT_MAX, dragStartWidth + ev.clientX - dragStartX));
		};
		const up = () => {
			document.removeEventListener('mousemove', move);
			document.removeEventListener('mouseup', up);
			saveState();
		};
		document.addEventListener('mousemove', move);
		document.addEventListener('mouseup', up);
		e.preventDefault();
	}
</script>

<div class="flex h-full overflow-hidden select-text" style="color:{textPrimary};">
	<nav
		class="flex-shrink-0 overflow-y-auto py-2"
		style="width:{leftWidth}px; background:{panelBg}; border-right:1px solid {borderColor};"
		aria-label={m.docbuilder_tree_label()}
	>
		{#each tree as cat (cat.id)}
			<div class="mb-0.5 px-2">
				<button
					class="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-2"
					style="color:{textMuted}; font-size:14px; font-weight:500;"
					onclick={() => {
						expanded[cat.id] = !expanded[cat.id];
						saveState();
					}}
				>
					{#if expanded[cat.id]}
						<FolderOpenIcon class="flex-shrink-0" style="width:16px; height:16px;" />
					{:else}
						<FolderIcon class="flex-shrink-0" style="width:16px; height:16px;" />
					{/if}
					<span class="flex-1 truncate text-left">{cat.label}</span>
					<span style="font-size:12px;">{cat.children?.length ?? 0}</span>
					<ChevronDownIcon
						class="flex-shrink-0 transition-transform duration-200"
						style="width:14px; height:14px; transform: rotate({expanded[cat.id]
							? '180deg'
							: '0deg'});"
					/>
				</button>
				{#if expanded[cat.id]}
					<div class="mt-0.5 mb-1 ml-3" style="border-left:2px solid {borderColor};">
						{#each cat.children ?? [] as leaf (leaf.id)}
							<button
								class="flex w-full cursor-pointer items-center gap-2 px-3 py-1.5 text-left"
								style="font-size:13px; color:{selected?.doc_id === leaf.doc_id
									? accent
									: textMuted}; background:{selected?.doc_id === leaf.doc_id
									? accentTint
									: 'transparent'};"
								onclick={() => select(leaf)}
							>
								{#if leaf.kind === 'pdf'}
									<FileIcon class="flex-shrink-0" style="width:14px; height:14px;" />
								{:else}
									<FileTextIcon class="flex-shrink-0" style="width:14px; height:14px;" />
								{/if}
								<span class="truncate">{leaf.label}</span>
							</button>
						{/each}
					</div>
				{/if}
			</div>
		{/each}
		{#if pending > 0}
			<p class="px-4 py-2" style="font-size:12px; color:{textMuted};">
				{m.docbuilder_pending({ count: pending })}
			</p>
		{/if}
	</nav>

	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="flex-shrink-0 cursor-col-resize select-none"
		style="width:4px; background:{borderColor};"
		onmousedown={startDrag}
		title={m.docbuilder_drag_to_resize()}
	></div>

	<div class="min-w-0 flex-1 overflow-y-auto" bind:this={contentEl} onscroll={saveState}>
		{#if loading}
			<p class="p-6" style="color:{textMuted};">{m.docbuilder_loading()}</p>
		{:else if error}
			<p class="p-6" style="color:#EF4444;">{error}</p>
		{:else if tree.length === 0}
			<p class="p-6" style="color:{textMuted};">{m.docbuilder_empty()}</p>
		{:else if !selected}
			<p class="p-6" style="color:{textMuted};">{m.docbuilder_select_document()}</p>
		{:else if selected.kind === 'pdf' && selected.doc_id}
			<iframe
				class="h-full w-full border-0"
				title={selected.label}
				src={docGroupFileUrl(group, selected.doc_id)}
			></iframe>
		{:else}
			<div class="p-6">
				<div class="mb-3 flex justify-end">
					<a
						class="rounded-md px-3 py-1"
						style="font-size:12px; color:{accent}; border:1px solid {borderColor};"
						href={docGroupPdfUrl(group, selected.doc_id!)}
						target="_blank"
						rel="noopener">{m.docbuilder_export_pdf()}</a
					>
				</div>
				<MarkdownViewer source={markdown} breaks={false} />
			</div>
		{/if}
	</div>
</div>

<script lang="ts">
	// The Chat and Dialog tabs of a page's chat (KnowledgeStore
	// doc-repo/code_modules/interpreter-chat-panel.md). Messages are sent from
	// the page's chat-composer, which shares this page instance's chat.
	import { onDestroy, tick } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import HarnessDialog from '$lib/components/shared-ui/harness-dialog.svelte';
	import HistoryIcon from '@lucide/svelte/icons/history';
	import XIcon from '@lucide/svelte/icons/x';
	import { acquireChat, releaseChat } from '../chat-session.svelte';
	import { getRuntime } from '../runtime.svelte';
	import type { ElementProps } from './index';

	let { instanceId, element }: ElementProps = $props();

	type Tab = 'chat' | 'dialog';
	const TAB_IDS: Tab[] = ['chat', 'dialog'];

	const rt = getRuntime();
	const chat = acquireChat(rt, instanceId, rt.editorPreview);
	onDestroy(() => releaseChat(rt, instanceId));

	const d = $derived(element.desc);
	const tabs = $derived(((d.tabs ?? TAB_IDS) as string[]).filter((t): t is Tab => TAB_IDS.includes(t as Tab)));
	let chosen = $state<Tab | null>(null);
	const current = $derived<Tab>(
		chosen && tabs.includes(chosen) ? chosen : ((d.initial_tab as Tab | undefined) ?? tabs[0] ?? 'chat')
	);

	const id = $props.id();
	const tabLabel = (tab: Tab) => (tab === 'chat' ? m.wpi_chat_tab_chat() : m.wpi_chat_tab_dialog());

	// The shared conversation view draws white text on its accent, so the accent
	// is a fixed indigo that reads in both themes; the rest follows the theme.
	const colors = {
		cardBg: 'var(--card)',
		surface2: 'var(--muted)',
		borderColor: 'var(--border)',
		accent: '#4F46E5',
		accentTint: 'rgba(79, 70, 229, 0.12)',
		textPrimary: 'var(--foreground)',
		textSecondary: 'var(--muted-foreground)',
		textMuted: 'var(--muted-foreground)'
	};

	const active = $derived(chat.active);
	const items = $derived(active?.items ?? []);
	const running = $derived(!!active?.running);
	const summary = $derived(
		items.filter((item) => item.kind === 'user' || item.kind === 'assistant' || item.kind === 'tool')
	);

	function onTabKey(event: KeyboardEvent, index: number) {
		const last = tabs.length - 1;
		const next =
			event.key === 'ArrowRight' ? (index === last ? 0 : index + 1)
			: event.key === 'ArrowLeft' ? (index === 0 ? last : index - 1)
			: event.key === 'Home' ? 0
			: event.key === 'End' ? last
			: -1;
		if (next < 0) return;
		event.preventDefault();
		chosen = tabs[next];
		void tick().then(() => document.getElementById(`${id}-tab-${tabs[next]}`)?.focus());
	}

	// Keeps the conversation at its end while the user is there; a user who
	// has scrolled up keeps their place.
	let scroller = $state<HTMLElement>();
	let atEnd = true;
	$effect.pre(() => {
		void items.length;
		void (items.at(-1) as { text?: string } | undefined)?.text;
		if (scroller) atEnd = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 24;
	});
	$effect(() => {
		void items.length;
		void (items.at(-1) as { text?: string } | undefined)?.text;
		if (scroller && atEnd) scroller.scrollTop = scroller.scrollHeight;
	});

	// Announces when a reply starts and ends, not the streamed text.
	let status = $state('');
	let wasRunning = false;
	$effect(() => {
		if (running && !wasRunning) status = m.wpi_chat_replying();
		else if (!running && wasRunning) status = m.wpi_chat_reply_finished();
		wasRunning = running;
	});
</script>

<div class="chat-panel bg-card text-card-foreground flex h-full min-h-0 w-full min-w-0 flex-col overflow-hidden rounded-lg border select-text">
	{#if tabs.length > 1}
		<div role="tablist" aria-label={m.wpi_chat_panel()} class="flex gap-2 border-b p-2">
			{#each tabs as tab, index (tab)}
				<button
					type="button"
					role="tab"
					id="{id}-tab-{tab}"
					aria-selected={current === tab}
					aria-controls="{id}-panel"
					tabindex={current === tab ? 0 : -1}
					onclick={() => (chosen = tab)}
					onkeydown={(e) => onTabKey(e, index)}
					class="cursor-pointer rounded-lg px-3 py-1.5 text-xs font-semibold {current === tab
						? 'bg-accent text-accent-foreground'
						: 'text-muted-foreground'}"
				>
					{tabLabel(tab)}
				</button>
			{/each}
		</div>
	{/if}

	<div
		id="{id}-panel"
		role={tabs.length > 1 ? 'tabpanel' : 'region'}
		aria-labelledby={tabs.length > 1 ? `${id}-tab-${current}` : undefined}
		aria-label={tabs.length > 1 ? undefined : tabLabel(current)}
		class="flex min-h-0 flex-1 flex-col"
	>
		{#if current === 'chat'}
			<div class="flex items-center gap-2 border-b px-2 py-1.5">
				<select
					value={active?.id ?? ''}
					disabled={!chat.visible.length}
					onchange={(e) => chat.selectTab(e.currentTarget.value)}
					aria-label={m.wpi_chat_session()}
					class="border-input bg-background min-w-0 flex-1 truncate rounded-md border px-2 py-1 text-xs disabled:opacity-60"
				>
					{#each chat.visible as tab (tab.id)}
						<option value={tab.id}>{tab.title || m.wpi_chat_new_session()}</option>
					{:else}
						<option value="">{m.wpi_chat_no_open_sessions()}</option>
					{/each}
				</select>
				<DropdownMenu.Root onOpenChange={(open) => open && chat.loadHistory()}>
					<DropdownMenu.Trigger
						disabled={chat.preview || !chat.harness}
						class="hover:bg-accent inline-flex cursor-pointer items-center gap-1 rounded-md border px-2 py-1 text-xs disabled:cursor-default disabled:opacity-60"
					>
						<HistoryIcon class="h-3.5 w-3.5" />
						{m.wpi_chat_history()}
					</DropdownMenu.Trigger>
					<DropdownMenu.Content class="max-h-96 overflow-y-auto">
						{#each chat.history as entry (entry.sessionId)}
							<DropdownMenu.Item onclick={() => chat.openHistory(entry)}>
								<div class="flex max-w-[320px] flex-col">
									<span class="truncate">{entry.title || entry.firstPrompt || entry.sessionId}</span>
									<span class="text-[10px] opacity-60">{new Date(entry.lastModified).toLocaleString()}</span>
								</div>
							</DropdownMenu.Item>
						{:else}
							<DropdownMenu.Item disabled>{m.wpi_chat_no_sessions()}</DropdownMenu.Item>
						{/each}
					</DropdownMenu.Content>
				</DropdownMenu.Root>
				<button
					type="button"
					disabled={!active}
					onclick={() => active && chat.closeTab(active.id)}
					aria-label={m.wpi_chat_close_session()}
					title={m.wpi_chat_close_session()}
					class="hover:bg-accent cursor-pointer rounded-md border p-1 disabled:cursor-default disabled:opacity-60"
				>
					<XIcon class="h-3.5 w-3.5" />
				</button>
			</div>
			<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
			<div
				bind:this={scroller}
				tabindex="0"
				role="region"
				aria-label={m.wpi_chat_conversation()}
				class="min-h-0 flex-1 overflow-y-auto px-3 py-3"
				style="scrollbar-width:thin;"
			>
				{#if items.length || running}
					<HarnessDialog
						{items}
						{running}
						{colors}
						onDecide={(requestId, decision) => chat.decide(requestId, decision)}
					/>
				{:else}
					<p class="text-muted-foreground text-sm">{m.wpi_chat_empty()}</p>
				{/if}
			</div>
		{:else}
			<div class="min-h-0 flex-1 space-y-2 overflow-y-auto p-3" style="scrollbar-width:thin;">
				{#each summary as item (item.id)}
					<div class="bg-muted rounded-lg border px-3 py-2 text-xs">
						<div class="text-muted-foreground mb-1 font-semibold tracking-wide uppercase">
							{item.kind === 'tool' ? item.name : item.kind === 'user' ? m.wpi_chat_role_user() : m.wpi_chat_role_assistant()}
						</div>
						{#if item.kind !== 'tool'}
							<div class="text-muted-foreground line-clamp-3">{item.text}</div>
						{/if}
					</div>
				{:else}
					<p class="text-muted-foreground text-xs">{m.wpi_chat_no_dialog()}</p>
				{/each}
			</div>
		{/if}
	</div>
	<p class="sr-only" role="status">{status}</p>
</div>

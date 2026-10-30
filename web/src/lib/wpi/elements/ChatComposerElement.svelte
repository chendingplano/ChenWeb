<script lang="ts">
	// The message box and tool bar of a page's chat (KnowledgeStore
	// doc-repo/code_modules/interpreter-chat-composer.md). Replies appear in the
	// page's chat-panel, which shares this page instance's chat.
	import { onDestroy } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import BotIcon from '@lucide/svelte/icons/bot';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import SendHorizontalIcon from '@lucide/svelte/icons/send-horizontal';
	import SquareIcon from '@lucide/svelte/icons/square';
	import { CHAT_AGENTS, acquireChat, releaseChat } from '../chat-session.svelte';
	import { getRuntime } from '../runtime.svelte';
	import { localized } from '../text';
	import type { ElementProps } from './index';

	let { instanceId, element }: ElementProps = $props();

	const DEFAULT_TOOLBAR = ['agent', 'model', 'prompt', 'text_editor', 'new_session'];

	const rt = getRuntime();
	const chat = acquireChat(rt, instanceId, rt.editorPreview);
	onDestroy(() => releaseChat(rt, instanceId));

	const d = $derived(element.desc);
	const toolbar = $derived(d.toolbar ?? DEFAULT_TOOLBAR);
	$effect.pre(() => {
		chat.config = { agents: d.agents ?? CHAT_AGENTS, defaultAgent: d.default_agent, defaultModel: d.default_model };
	});
	$effect(() => {
		void chat.loadModels(chat.harness);
	});

	const id = $props.id();
	let draft = $state('');
	// Escape hides the slash command list until the draft is edited again.
	let slashHidden = $state(false);
	let promptOpen = $state(false);
	let promptSearch = $state('');
	let editorOpen = $state(false);
	let editorValue = $state('');

	const live = $derived(!chat.preview && chat.loaded);
	const running = $derived(!!chat.active?.running);
	const canSend = $derived(live && !!chat.harness && draft.trim() !== '' && !running);
	const modelState = $derived(chat.modelStatus[chat.harness]);
	const slashMatches = $derived(
		d.slash_commands !== false && !slashHidden && draft.startsWith('/')
			? chat.slashCommands.filter((command) => command.toLowerCase().includes(draft.toLowerCase()))
			: []
	);
	const prompts = $derived(
		[...chat.prompts]
			.sort((a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime())
			.filter((prompt) =>
				`${prompt.title}\n${prompt.content}`.toLowerCase().includes(promptSearch.toLowerCase())
			)
	);

	function send() {
		if (!canSend) return;
		const text = draft.trim();
		draft = '';
		void chat.send(text);
	}

	function onKeydown(event: KeyboardEvent) {
		if (event.key === 'Enter' && (event.ctrlKey || event.metaKey) && !event.isComposing) {
			event.preventDefault();
			send();
		} else if (event.key === 'Escape' && slashMatches.length) {
			slashHidden = true;
		}
	}
</script>

<div class="chat-composer flex w-full min-w-0 flex-col gap-2 select-text">
	<label for={id} class="sr-only">{m.wpi_chat_message()}</label>
	<textarea
		{id}
		bind:value={draft}
		rows={d.rows ?? 3}
		placeholder={localized(d.placeholder) || m.wpi_chat_placeholder()}
		onkeydown={onKeydown}
		oninput={() => (slashHidden = false)}
		class="border-input bg-background text-foreground placeholder:text-muted-foreground focus-visible:ring-ring w-full resize-none rounded-lg border px-3 py-2 text-sm outline-none focus-visible:ring-2"
	></textarea>

	{#if slashMatches.length}
		<ul class="bg-popover text-popover-foreground w-full max-w-80 rounded-lg border p-1 shadow-md" aria-label={m.wpi_chat_slash_commands()}>
			{#each slashMatches as command (command)}
				<li>
					<button
						type="button"
						class="hover:bg-accent focus-visible:bg-accent w-full rounded px-2 py-1.5 text-left text-xs outline-none"
						onclick={() => (draft = `${command} `)}
					>
						{command}
					</button>
				</li>
			{/each}
		</ul>
	{/if}

	<div class="flex flex-wrap items-center gap-2" role="group" aria-label={m.wpi_chat_controls()}>
		{#each toolbar as control (control)}
			{#if control === 'agent'}
				<DropdownMenu.Root>
					<DropdownMenu.Trigger
						disabled={!live}
						class="bg-accent text-accent-foreground inline-flex cursor-pointer items-center gap-1 rounded-lg border px-3 py-1.5 text-xs font-semibold disabled:cursor-default disabled:opacity-60"
						aria-label={m.wpi_chat_agent_named({ name: chat.agent || m.wpi_chat_select_agent() })}
					>
						<BotIcon class="h-3.5 w-3.5" />
						{chat.agent || m.wpi_chat_select_agent()}
						<ChevronDownIcon class="h-3.5 w-3.5" />
					</DropdownMenu.Trigger>
					<DropdownMenu.Content>
						{#each chat.agents as agent (agent)}
							<DropdownMenu.Item onclick={() => chat.selectAgent(agent)}>{agent}</DropdownMenu.Item>
						{:else}
							<DropdownMenu.Item disabled>{m.wpi_chat_no_agents()}</DropdownMenu.Item>
						{/each}
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			{:else if control === 'model'}
				<select
					value={chat.model}
					disabled={!live || !chat.modelChoices.length}
					onchange={(e) => chat.selectModel(e.currentTarget.value)}
					aria-label={m.wpi_chat_model()}
					class="border-input bg-background text-foreground rounded-lg border px-3 py-1.5 text-xs disabled:opacity-60"
				>
					{#if modelState === 'loading'}
						<option value={chat.model} disabled>{m.wpi_chat_models_loading()}</option>
					{:else if modelState === 'error' || (live && !chat.modelChoices.length)}
						<option value={chat.model} disabled>{m.wpi_chat_models_unavailable()}</option>
					{:else}
						{#each chat.modelChoices as choice (choice.value)}
							<option value={choice.value} title={choice.description}>{choice.label}</option>
						{/each}
					{/if}
				</select>
			{:else if control === 'prompt'}
				<button
					type="button"
					disabled={!live}
					onclick={() => (promptOpen = true)}
					class="bg-background text-foreground hover:bg-accent cursor-pointer rounded-lg border px-3 py-1.5 text-xs font-semibold disabled:cursor-default disabled:opacity-60"
				>
					{m.wpi_chat_prompt()}
				</button>
			{:else if control === 'text_editor'}
				<button
					type="button"
					disabled={!live}
					onclick={() => {
						editorValue = draft;
						editorOpen = true;
					}}
					class="bg-background text-foreground hover:bg-accent inline-flex cursor-pointer items-center gap-1 rounded-lg border px-3 py-1.5 text-xs font-semibold disabled:cursor-default disabled:opacity-60"
				>
					<FileTextIcon class="h-3.5 w-3.5" />
					{m.wpi_chat_text_editor()}
				</button>
			{:else if control === 'new_session'}
				<button
					type="button"
					disabled={!live || !chat.harness}
					onclick={() => chat.newSession()}
					class="bg-background text-foreground hover:bg-accent cursor-pointer rounded-lg border px-3 py-1.5 text-xs font-semibold disabled:cursor-default disabled:opacity-60"
				>
					{m.wpi_chat_new_session()}
				</button>
			{/if}
		{/each}

		<div class="ml-auto">
			{#if running}
				<button
					type="button"
					onclick={() => chat.stop()}
					class="bg-background text-foreground hover:bg-accent inline-flex cursor-pointer items-center gap-1 rounded-lg border px-3 py-1.5 text-xs font-semibold"
				>
					<SquareIcon class="h-3.5 w-3.5" />
					{m.wpi_chat_stop()}
				</button>
			{:else}
				<button
					type="button"
					disabled={!canSend}
					onclick={send}
					title={m.wpi_chat_send_hint()}
					class="bg-primary text-primary-foreground inline-flex cursor-pointer items-center gap-1 rounded-lg px-3 py-1.5 text-xs font-semibold disabled:cursor-default disabled:opacity-50"
				>
					<SendHorizontalIcon class="h-3.5 w-3.5" />
					{m.wpi_chat_send()}
				</button>
			{/if}
		</div>
	</div>
</div>

<Dialog.Root bind:open={promptOpen}>
	<Dialog.Content class="max-w-xl select-text">
		<Dialog.Header>
			<Dialog.Title>{m.wpi_chat_prompt_selection()}</Dialog.Title>
		</Dialog.Header>
		<input
			type="search"
			bind:value={promptSearch}
			placeholder={m.wpi_chat_search_prompts()}
			aria-label={m.wpi_chat_search_prompts()}
			class="border-input bg-background w-full rounded-lg border px-3 py-2 text-sm outline-none"
		/>
		<div class="max-h-80 space-y-2 overflow-y-auto">
			{#each prompts as prompt (prompt.id)}
				<button
					type="button"
					class="bg-muted hover:bg-accent w-full cursor-pointer rounded-lg border px-3 py-2 text-left"
					onclick={() => {
						draft = prompt.content;
						promptOpen = false;
					}}
				>
					<div class="text-sm font-semibold">{prompt.title}</div>
					<div class="text-muted-foreground text-xs">{prompt.content}</div>
				</button>
			{:else}
				<p class="text-muted-foreground text-sm">{m.wpi_chat_no_prompts()}</p>
			{/each}
		</div>
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={editorOpen}>
	<Dialog.Content class="max-w-2xl select-text">
		<Dialog.Header>
			<Dialog.Title>{m.wpi_chat_text_editor()}</Dialog.Title>
		</Dialog.Header>
		<textarea
			bind:value={editorValue}
			rows={14}
			aria-label={m.wpi_chat_message()}
			class="border-input bg-background w-full rounded-lg border p-3 text-sm outline-none"
		></textarea>
		<Dialog.Footer>
			<button type="button" class="bg-background hover:bg-accent cursor-pointer rounded-lg border px-3 py-1.5 text-xs font-semibold" onclick={() => (editorOpen = false)}>
				{m.wpi_chat_cancel()}
			</button>
			<button
				type="button"
				class="bg-primary text-primary-foreground cursor-pointer rounded-lg px-3 py-1.5 text-xs font-semibold"
				onclick={() => {
					draft = editorValue;
					editorOpen = false;
				}}
			>
				{m.wpi_chat_apply()}
			</button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

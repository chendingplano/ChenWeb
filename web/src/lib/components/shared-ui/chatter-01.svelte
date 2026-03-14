<script lang="ts">
	import { onMount } from 'svelte';
	import { chatterService, type ChatterDialogItem, type ChatterSettings } from '$lib/services/chatterService';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';

	import BotIcon from '@lucide/svelte/icons/bot';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import MicIcon from '@lucide/svelte/icons/mic';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import SparklesIcon from '@lucide/svelte/icons/sparkles';
	import SendHorizontalIcon from '@lucide/svelte/icons/send-horizontal';
	import XIcon from '@lucide/svelte/icons/x';

	type ChatSession = {
		id: string;
		title: string;
		dialogs: ChatterDialogItem[];
	};

	const defaultSettings: ChatterSettings = {
		agents: ['OpenClaw', 'Claude Code', 'Codex', 'Qwen Code', 'OpenCode', 'pi'],
		models: ['ChatGPT 5.4', 'Claude Sonnet 4.6', 'GPT-4o', 'Gemini Pro 2.5', 'Qwen3-Coder'],
		attachments: ['Photos and Files', 'Recent Files', '---', 'Create an image', 'Deep Research'],
		skills: ['Create Skill', 'superpowers', 'docx', 'pptx', 'pdf'],
		resultOptions: ['Text', 'Markdown', 'JSON', 'Web Page'],
		slashCommands: ['/help', '/summarize', '/translate', '/rewrite', '/table', '/extract']
	};

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let settings = $state<ChatterSettings>(defaultSettings);
	let prompts = $state<{ id: string; title: string; content: string; updatedAt: string }[]>([]);

	let selectedAgent = $state(defaultSettings.agents[0]);
	let selectedModel = $state(defaultSettings.models[0]);
	let selectedResult = $state(defaultSettings.resultOptions[0]);
	let selectedSkill = $state(defaultSettings.skills[1]);

	let infoWidth = $state(360);
	const infoMin = 280;
	const infoMax = 560;
	let isDraggingInfo = false;
	let dragStartX = 0;
	let dragStartWidth = 0;

	let sessions = $state<ChatSession[]>([
		{ id: 'local-1', title: 'New Session', dialogs: [] }
	]);
	let activeSessionId = $state('local-1');
	let draft = $state('');
	let promptSearch = $state('');
	let promptDialogOpen = $state(false);
	let textEditorOpen = $state(false);
	let textEditorValue = $state('');
	let infoTab = $state<'dialog' | 'settings'>('dialog');

	const chatBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	const cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	const borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	const accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	const textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	const textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');

	const activeSession = $derived(
		sessions.find((session) => session.id === activeSessionId) ?? sessions[0]
	);

	const filteredPrompts = $derived(
		[...prompts]
			.sort((a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime())
			.filter(
				(prompt) =>
					prompt.title.toLowerCase().includes(promptSearch.toLowerCase()) ||
					prompt.content.toLowerCase().includes(promptSearch.toLowerCase())
			)
	);

	const slashMatches = $derived(
		draft.startsWith('/')
			? settings.slashCommands.filter((command) =>
					command.toLowerCase().includes(draft.toLowerCase())
				)
			: []
	);

	onMount(async () => {
		await Promise.all([loadSettings(), loadPrompts(), loadSlashCommands(), loadSessions()]);
	});

	function startInfoDrag(e: MouseEvent) {
		isDraggingInfo = true;
		dragStartX = e.clientX;
		dragStartWidth = infoWidth;
		document.addEventListener('mousemove', onMouseMove);
		document.addEventListener('mouseup', onMouseUp);
		e.preventDefault();
	}

	function onMouseMove(e: MouseEvent) {
		if (!isDraggingInfo) return;
		const delta = dragStartX - e.clientX;
		infoWidth = Math.max(infoMin, Math.min(infoMax, dragStartWidth + delta));
	}

	function onMouseUp() {
		isDraggingInfo = false;
		document.removeEventListener('mousemove', onMouseMove);
		document.removeEventListener('mouseup', onMouseUp);
	}

	async function loadSettings() {
		try {
			const res = await chatterService.getSettings();
			settings = {
				...defaultSettings,
				...res.settings
			};
			selectedAgent = settings.agents[0] ?? selectedAgent;
			selectedModel = settings.models[0] ?? selectedModel;
			selectedResult = settings.resultOptions[0] ?? selectedResult;
			selectedSkill = settings.skills[0] ?? selectedSkill;
		} catch {
			settings = defaultSettings;
		}
	}

	async function loadPrompts() {
		try {
			const res = await chatterService.getPrompts();
			prompts = res.prompts;
		} catch {
			prompts = [
				{
					id: 'prompt-local-1',
					title: 'Summarize this conversation',
					content: 'Summarize this conversation in bullet points.',
					updatedAt: '2026-03-13T22:00:00Z'
				},
				{
					id: 'prompt-local-2',
					title: 'Write implementation plan',
					content: 'Create a step-by-step implementation plan with risks.',
					updatedAt: '2026-03-13T20:00:00Z'
				}
			];
		}
	}

	async function loadSlashCommands() {
		try {
			const res = await chatterService.getSlashCommands();
			if (res.commands?.length) {
				settings = { ...settings, slashCommands: res.commands };
			}
		} catch {
			// use defaults
		}
	}

	async function loadSessions() {
		try {
			const res = await chatterService.listSessions();
			if (res.sessions?.length) {
				sessions = res.sessions.map((session) => ({
					id: session.id,
					title: session.title,
					dialogs: []
				}));
				activeSessionId = sessions[0].id;
				await loadDialogs(sessions[0].id);
			}
		} catch {
			// keep local fallback
		}
	}

	async function loadDialogs(sessionId: string) {
		try {
			const res = await chatterService.getDialogs(sessionId);
			sessions = sessions.map((session) =>
				session.id === sessionId ? { ...session, dialogs: res.dialogs } : session
			);
		} catch {
			// keep local fallback
		}
	}

	async function newSession() {
		try {
			const res = await chatterService.createSession();
			const next = { id: res.session.id, title: res.session.title, dialogs: [] as ChatterDialogItem[] };
			sessions = [...sessions, next];
			activeSessionId = next.id;
		} catch {
			const id = `local-${Date.now()}`;
			const next = { id, title: `Session ${sessions.length + 1}`, dialogs: [] as ChatterDialogItem[] };
			sessions = [...sessions, next];
			activeSessionId = id;
		}
	}

	function closeSession(sessionId: string) {
		if (sessions.length <= 1) return;
		const nextSessions = sessions.filter((session) => session.id !== sessionId);
		sessions = nextSessions;
		if (activeSessionId === sessionId) {
			activeSessionId = nextSessions[0].id;
		}
	}

	function addDialogItem(dialog: ChatterDialogItem) {
		sessions = sessions.map((session) =>
			session.id === activeSessionId ? { ...session, dialogs: [...session.dialogs, dialog] } : session
		);
	}

	async function sendMessage() {
		const input = draft.trim();
		if (!input || !activeSession) return;

		addDialogItem({
			id: `user-${Date.now()}`,
			role: 'user',
			content: input,
			createdAt: new Date().toISOString()
		});

		draft = '';

		try {
			const res = await chatterService.sendMessage(activeSession.id, {
				input,
				model: selectedModel,
				agent: selectedAgent,
				resultType: selectedResult
			});
			addDialogItem(res.dialog);
		} catch {
			addDialogItem({
				id: `assistant-${Date.now()}`,
				role: 'assistant',
				content: `Stub response from ${selectedAgent} with ${selectedModel}.`,
				createdAt: new Date().toISOString()
			});
		}
	}

	function applyPrompt(content: string) {
		draft = content;
		promptDialogOpen = false;
	}

	function insertSlashCommand(command: string) {
		draft = `${command} `;
	}

	async function saveSettings() {
		await chatterService.updateSettings(settings).catch(() => {});
	}
</script>

<div class="relative flex h-full min-h-0" style="background:{chatBg}; color:{textPrimary};">
	<div class="flex min-w-0 flex-1 flex-col overflow-hidden">
		<div class="flex flex-wrap items-center gap-2 border-b px-4 py-3" style="border-color:{borderColor};">
			<DropdownMenu.Root>
				<DropdownMenu.Trigger
					class="rounded-lg px-3 py-1.5 text-xs font-semibold cursor-pointer"
					style="background:{accentTint}; color:{accent}; border:1px solid {borderColor};"
				>
					<BotIcon class="mr-1 inline h-3.5 w-3.5" /> {selectedAgent}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content>
					{#each settings.agents as agent}
						<DropdownMenu.Item onclick={() => (selectedAgent = agent)}>{agent}</DropdownMenu.Item>
					{/each}
				</DropdownMenu.Content>
			</DropdownMenu.Root>

			<select
				bind:value={selectedModel}
				class="rounded-lg px-3 py-1.5 text-xs"
				style="background:{cardBg}; color:{textPrimary}; border:1px solid {borderColor};"
			>
				{#each settings.models as model}
					<option value={model}>{model}</option>
				{/each}
			</select>

			<DropdownMenu.Root>
				<DropdownMenu.Trigger
					class="rounded-lg px-3 py-1.5 text-xs font-semibold cursor-pointer"
					style="background:{cardBg}; color:{textPrimary}; border:1px solid {borderColor};"
				>
					<PlusIcon class="mr-1 inline h-3.5 w-3.5" /> Attach
				</DropdownMenu.Trigger>
				<DropdownMenu.Content>
					{#each settings.attachments as item}
						{#if item === '---'}
							<DropdownMenu.Separator />
						{:else}
							<DropdownMenu.Item>{item}</DropdownMenu.Item>
						{/if}
					{/each}
				</DropdownMenu.Content>
			</DropdownMenu.Root>

			<DropdownMenu.Root>
				<DropdownMenu.Trigger
					class="rounded-lg px-3 py-1.5 text-xs font-semibold cursor-pointer"
					style="background:{cardBg}; color:{textPrimary}; border:1px solid {borderColor};"
				>
					<SparklesIcon class="mr-1 inline h-3.5 w-3.5" /> {selectedSkill}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content>
					{#each settings.skills as item}
						<DropdownMenu.Item onclick={() => (selectedSkill = item)}>{item}</DropdownMenu.Item>
					{/each}
				</DropdownMenu.Content>
			</DropdownMenu.Root>

			<select
				bind:value={selectedResult}
				class="rounded-lg px-3 py-1.5 text-xs"
				style="background:{cardBg}; color:{textPrimary}; border:1px solid {borderColor};"
			>
				{#each settings.resultOptions as resultOption}
					<option value={resultOption}>{resultOption}</option>
				{/each}
			</select>

			<button
				class="rounded-lg px-3 py-1.5 text-xs font-semibold cursor-pointer"
				style="background:{cardBg}; color:{textPrimary}; border:1px solid {borderColor};"
				onclick={() => (promptDialogOpen = true)}
			>
				Prompt
			</button>

			<button
				class="rounded-lg px-3 py-1.5 text-xs font-semibold cursor-pointer"
				style="background:{cardBg}; color:{textPrimary}; border:1px solid {borderColor};"
			>
				<MicIcon class="mr-1 inline h-3.5 w-3.5" /> Dictate
			</button>

			<button
				class="rounded-lg px-3 py-1.5 text-xs font-semibold cursor-pointer"
				style="background:{cardBg}; color:{textPrimary}; border:1px solid {borderColor};"
				onclick={() => {
					textEditorValue = draft;
					textEditorOpen = true;
				}}
			>
				<FileTextIcon class="mr-1 inline h-3.5 w-3.5" /> Text
			</button>

			<button
				class="rounded-lg px-3 py-1.5 text-xs font-semibold cursor-pointer"
				style="background:{accent}; color:white; border:1px solid {accent};"
				onclick={newSession}
			>
				New Session
			</button>
		</div>

		<div class="flex items-center gap-1 overflow-x-auto border-b px-3 py-2" style="border-color:{borderColor};">
			{#each sessions as session (session.id)}
				<button
					class="flex items-center gap-2 rounded-lg px-3 py-1.5 text-xs cursor-pointer"
					style="background:{activeSessionId === session.id ? accentTint : cardBg}; border:1px solid {borderColor}; color:{activeSessionId === session.id ? accent : textSecondary};"
					onclick={() => {
						activeSessionId = session.id;
						loadDialogs(session.id);
					}}
				>
					<span class="max-w-[140px] truncate">{session.title}</span>
					<XIcon class="h-3 w-3" onclick={(e) => { e.stopPropagation(); closeSession(session.id); }} />
				</button>
			{/each}
		</div>

		<div class="flex-1 overflow-y-auto px-4 py-4" style="scrollbar-width:thin;">
			{#if activeSession?.dialogs?.length}
				<div class="space-y-3">
					{#each activeSession.dialogs as dialog (dialog.id)}
						<div class={dialog.role === 'user' ? 'flex justify-end' : 'flex justify-start'}>
							<div
								class="max-w-[78%] rounded-xl px-3 py-2 text-sm"
								style="background:{dialog.role === 'user' ? accent : cardBg}; color:{dialog.role === 'user' ? 'white' : textPrimary}; border:1px solid {dialog.role === 'user' ? accent : borderColor};"
							>
								<div>{dialog.content}</div>
							</div>
						</div>
					{/each}
				</div>
			{:else}
				<div class="flex h-full items-center justify-center text-sm" style="color:{textMuted};">
					Start chatting. Type `/` to use slash commands.
				</div>
			{/if}
		</div>

		<div class="relative border-t px-4 py-3" style="border-color:{borderColor};">
			<textarea
				bind:value={draft}
				rows={3}
				placeholder="Ask anything... Use '/' for commands."
				class="w-full resize-none rounded-xl px-3 py-2 text-sm outline-none"
				style="background:{cardBg}; color:{textPrimary}; border:1px solid {borderColor};"
			></textarea>

			{#if slashMatches.length > 0}
				<div
					class="absolute bottom-[86px] left-4 z-20 w-[320px] rounded-xl p-1"
					style="background:{cardBg}; border:1px solid {borderColor}; box-shadow:0 10px 24px rgba(0,0,0,0.2);"
				>
					{#each slashMatches as command}
						<button
							class="block w-full rounded-md px-2 py-1.5 text-left text-xs cursor-pointer"
							style="color:{textSecondary};"
							onclick={() => insertSlashCommand(command)}
						>
							{command}
						</button>
					{/each}
				</div>
			{/if}

			<div class="mt-2 flex items-center justify-end">
				<button
					class="rounded-lg px-3 py-1.5 text-xs font-semibold cursor-pointer"
					style="background:{accent}; color:white; border:1px solid {accent};"
					onclick={sendMessage}
				>
					<SendHorizontalIcon class="mr-1 inline h-3.5 w-3.5" /> Send
				</button>
			</div>
		</div>
	</div>

	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="group flex w-1 cursor-col-resize items-center justify-center"
		style="background:{borderColor};"
		onmousedown={startInfoDrag}
	>
		<div class="h-8 w-0.5 opacity-60 group-hover:opacity-100" style="background:{accent};"></div>
	</div>

	<div class="flex flex-col overflow-hidden" style="width:{infoWidth}px; background:{cardBg}; border-left:1px solid {borderColor};">
		<div class="flex border-b p-2" style="border-color:{borderColor};">
			<button
				class="rounded-lg px-3 py-1.5 text-xs font-semibold cursor-pointer"
				style="background:{infoTab === 'dialog' ? accentTint : 'transparent'}; color:{infoTab === 'dialog' ? accent : textSecondary};"
				onclick={() => (infoTab = 'dialog')}
			>
				Dialog
			</button>
			<button
				class="ml-2 rounded-lg px-3 py-1.5 text-xs font-semibold cursor-pointer"
				style="background:{infoTab === 'settings' ? accentTint : 'transparent'}; color:{infoTab === 'settings' ? accent : textSecondary};"
				onclick={() => (infoTab = 'settings')}
			>
				Settings
			</button>
		</div>

		{#if infoTab === 'dialog'}
			<div class="flex-1 overflow-y-auto p-3" style="scrollbar-width:thin;">
				{#if activeSession?.dialogs?.length}
					<div class="space-y-2">
						{#each activeSession.dialogs as dialog (dialog.id)}
							<div class="rounded-lg px-3 py-2 text-xs" style="background:{surface2}; border:1px solid {borderColor};">
								<div class="mb-1 font-semibold uppercase tracking-wide" style="color:{textMuted};">
									{dialog.role}
								</div>
								<div style="color:{textSecondary};">{dialog.content}</div>
							</div>
						{/each}
					</div>
				{:else}
					<div class="text-xs" style="color:{textMuted};">No dialog history yet.</div>
				{/if}
			</div>
		{:else}
			<div class="flex-1 overflow-y-auto p-3 space-y-3" style="scrollbar-width:thin;">
				<div class="rounded-xl p-3" style="background:{surface2}; border:1px solid {borderColor};">
					<div class="mb-2 text-xs font-semibold" style="color:{textMuted};">Agent Selector List</div>
					<textarea value={settings.agents.join('\n')} rows={4} class="w-full rounded-lg p-2 text-xs"
						style="background:{cardBg}; color:{textPrimary}; border:1px solid {borderColor};"
						onchange={(e) => settings = { ...settings, agents: (e.currentTarget as HTMLTextAreaElement).value.split('\n').map((v) => v.trim()).filter(Boolean) }}
					></textarea>
				</div>

				<div class="rounded-xl p-3" style="background:{surface2}; border:1px solid {borderColor};">
					<div class="mb-2 text-xs font-semibold" style="color:{textMuted};">Model Selector List</div>
					<textarea value={settings.models.join('\n')} rows={4} class="w-full rounded-lg p-2 text-xs"
						style="background:{cardBg}; color:{textPrimary}; border:1px solid {borderColor};"
						onchange={(e) => settings = { ...settings, models: (e.currentTarget as HTMLTextAreaElement).value.split('\n').map((v) => v.trim()).filter(Boolean) }}
					></textarea>
				</div>

				<div class="rounded-xl p-3" style="background:{surface2}; border:1px solid {borderColor};">
					<div class="mb-2 text-xs font-semibold" style="color:{textMuted};">Attachment Selector List</div>
					<textarea value={settings.attachments.join('\n')} rows={5} class="w-full rounded-lg p-2 text-xs"
						style="background:{cardBg}; color:{textPrimary}; border:1px solid {borderColor};"
						onchange={(e) => settings = { ...settings, attachments: (e.currentTarget as HTMLTextAreaElement).value.split('\n').map((v) => v.trim()).filter(Boolean) }}
					></textarea>
				</div>

				<div class="rounded-xl p-3" style="background:{surface2}; border:1px solid {borderColor};">
					<div class="mb-2 text-xs font-semibold" style="color:{textMuted};">Plugin/Skill Selector List</div>
					<textarea value={settings.skills.join('\n')} rows={4} class="w-full rounded-lg p-2 text-xs"
						style="background:{cardBg}; color:{textPrimary}; border:1px solid {borderColor};"
						onchange={(e) => settings = { ...settings, skills: (e.currentTarget as HTMLTextAreaElement).value.split('\n').map((v) => v.trim()).filter(Boolean) }}
					></textarea>
				</div>

				<div class="rounded-xl p-3" style="background:{surface2}; border:1px solid {borderColor};">
					<div class="mb-2 text-xs font-semibold" style="color:{textMuted};">Result Options List</div>
					<textarea value={settings.resultOptions.join('\n')} rows={3} class="w-full rounded-lg p-2 text-xs"
						style="background:{cardBg}; color:{textPrimary}; border:1px solid {borderColor};"
						onchange={(e) => settings = { ...settings, resultOptions: (e.currentTarget as HTMLTextAreaElement).value.split('\n').map((v) => v.trim()).filter(Boolean) }}
					></textarea>
				</div>

				<button
					class="rounded-lg px-3 py-1.5 text-xs font-semibold cursor-pointer"
					style="background:{accent}; color:white; border:1px solid {accent};"
					onclick={saveSettings}
				>
					Save Settings
				</button>
			</div>
		{/if}
	</div>

	{#if promptDialogOpen}
		<div class="absolute inset-0 z-40 flex items-center justify-center" style="background:rgba(0,0,0,0.45);">
			<div class="w-[640px] max-w-[92vw] rounded-xl p-4" style="background:{cardBg}; border:1px solid {borderColor};">
				<div class="mb-2 text-sm font-semibold" style="color:{textPrimary};">Prompt Selection</div>
				<input
					type="text"
					placeholder="Search prompts..."
					bind:value={promptSearch}
					class="mb-3 w-full rounded-lg px-3 py-2 text-sm outline-none"
					style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				/>
				<div class="max-h-[320px] space-y-2 overflow-y-auto pr-1">
					{#each filteredPrompts as prompt (prompt.id)}
						<button
							class="w-full rounded-lg px-3 py-2 text-left cursor-pointer"
							style="background:{surface2}; border:1px solid {borderColor};"
							onclick={() => applyPrompt(prompt.content)}
						>
							<div class="text-sm font-semibold" style="color:{textPrimary};">{prompt.title}</div>
							<div class="text-xs" style="color:{textSecondary};">{prompt.content}</div>
						</button>
					{/each}
				</div>
				<div class="mt-3 flex justify-end">
					<button
						class="rounded-lg px-3 py-1.5 text-xs font-semibold cursor-pointer"
						style="background:{surface2}; color:{textSecondary}; border:1px solid {borderColor};"
						onclick={() => (promptDialogOpen = false)}
					>
						Close
					</button>
				</div>
			</div>
		</div>
	{/if}

	{#if textEditorOpen}
		<div class="absolute inset-0 z-40 flex items-center justify-center" style="background:rgba(0,0,0,0.45);">
			<div class="w-[720px] max-w-[95vw] rounded-xl p-4" style="background:{cardBg}; border:1px solid {borderColor};">
				<div class="mb-2 text-sm font-semibold" style="color:{textPrimary};">Text Editor</div>
				<textarea
					bind:value={textEditorValue}
					rows={14}
					class="w-full rounded-lg p-3 text-sm outline-none"
					style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				></textarea>
				<div class="mt-3 flex justify-end gap-2">
					<button
						class="rounded-lg px-3 py-1.5 text-xs font-semibold cursor-pointer"
						style="background:{surface2}; color:{textSecondary}; border:1px solid {borderColor};"
						onclick={() => (textEditorOpen = false)}
					>
						Cancel
					</button>
					<button
						class="rounded-lg px-3 py-1.5 text-xs font-semibold cursor-pointer"
						style="background:{accent}; color:white; border:1px solid {accent};"
						onclick={() => {
							draft = textEditorValue;
							textEditorOpen = false;
						}}
					>
						Apply
					</button>
				</div>
			</div>
		</div>
	{/if}
</div>

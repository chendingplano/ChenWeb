// The chat of one page instance, shared by its chat-composer and chat-panel
// elements (KnowledgeStore doc-repo/code_modules/interpreter-chat-composer.md,
// "Chat session"). It is runtime state, never part of the input doc: nothing
// here is saved or makes the page dirty. Only the harness-backed assistants are
// offered; a page chooses among them in its descriptor.
import { m } from '$lib/paraglide/messages.js';
import { chatterService, type ChatterPrompt } from '$lib/services/chatterService';
import {
	HARNESS_BY_AGENT,
	HarnessRequestError,
	decideHarnessPermission,
	getHarnessMessages,
	interruptHarnessTurn,
	listHarnessModels,
	listHarnessSessions,
	listHarnesses,
	streamHarnessTurn,
	type HarnessDecision,
	type HarnessSession
} from '$lib/services/harnessFrontendService';
import {
	applyEvent,
	itemsFromHistory,
	withDecision,
	type HarnessItem
} from '$lib/components/shared-ui/harness-conversation';
import { CHAT_AGENTS, offeredAgents, pick } from './chat-agents';

export { CHAT_AGENTS };

const FALLBACK_SLASH_COMMANDS = ['/help', '/summarize', '/translate', '/rewrite', '/table', '/extract'];

export type ChatConfig = { agents: string[]; defaultAgent?: string; defaultModel?: string };
export type ChatModel = { value: string; label: string; description?: string };

/** One open session of a harness; title is empty until its first message. */
export type ChatTab = {
	id: string;
	harness: string;
	title: string;
	sessionId?: string;
	items: HarnessItem[];
	running: boolean;
};

function errorText(agent: string, error: unknown): string {
	if (error instanceof HarnessRequestError && (error.status === 401 || error.status === 403)) {
		return m.wpi_chat_no_access();
	}
	return m.wpi_chat_send_failed({ agent, detail: error instanceof Error ? error.message : String(error) });
}

let tabCounter = 0;

export class ChatSession {
	config = $state<ChatConfig>({ agents: CHAT_AGENTS });
	// Harness names the server has configured; null until known or when the
	// list cannot be read, so the configured assistants are all offered.
	available = $state<string[] | null>(null);
	loaded = $state(false);
	chosenAgent = $state('');
	chosenModel = $state('');
	models = $state<Record<string, ChatModel[]>>({});
	modelStatus = $state<Record<string, 'loading' | 'error'>>({});
	tabs = $state<ChatTab[]>([]);
	activeId = $state('');
	history = $state<HarnessSession[]>([]);
	prompts = $state<ChatterPrompt[]>([]);
	slashCommands = $state<string[]>(FALLBACK_SLASH_COMMANDS);
	readonly preview: boolean;
	#streams = new Map<string, AbortController>();

	constructor(preview: boolean) {
		this.preview = preview;
		if (!preview) void this.#load();
		else this.loaded = true;
	}

	get agents(): string[] {
		return offeredAgents(this.config.agents, this.available);
	}
	get agent(): string {
		return pick(this.agents, this.chosenAgent, this.config.defaultAgent);
	}
	get harness(): string {
		return HARNESS_BY_AGENT[this.agent] ?? '';
	}
	get modelChoices(): ChatModel[] {
		return this.models[this.harness] ?? [];
	}
	get model(): string {
		return pick(
			this.modelChoices.map((choice) => choice.value),
			this.chosenModel,
			this.config.defaultModel
		);
	}
	get visible(): ChatTab[] {
		return this.tabs.filter((tab) => tab.harness === this.harness);
	}
	get active(): ChatTab | undefined {
		return this.visible.find((tab) => tab.id === this.activeId) ?? this.visible[0];
	}

	async #load() {
		const [harnesses, prompts, commands] = await Promise.allSettled([
			listHarnesses(),
			chatterService.getPrompts(),
			chatterService.getSlashCommands()
		]);
		if (harnesses.status === 'fulfilled') this.available = harnesses.value;
		this.prompts =
			prompts.status === 'fulfilled'
				? prompts.value.prompts
				: [
						{
							id: 'prompt-local-1',
							title: m.wpi_chat_prompt_summarize_title(),
							content: m.wpi_chat_prompt_summarize(),
							updatedAt: '2026-03-13T22:00:00Z'
						},
						{
							id: 'prompt-local-2',
							title: m.wpi_chat_prompt_plan_title(),
							content: m.wpi_chat_prompt_plan(),
							updatedAt: '2026-03-13T20:00:00Z'
						}
					];
		if (commands.status === 'fulfilled' && commands.value.commands?.length) {
			this.slashCommands = commands.value.commands;
		}
		this.loaded = true;
	}

	/** Loads the models of a harness once; the model menu shows the states. */
	async loadModels(harness: string) {
		if (this.preview || !harness || this.models[harness] || this.modelStatus[harness] === 'loading') return;
		this.modelStatus[harness] = 'loading';
		try {
			const models = await listHarnessModels(harness);
			this.models[harness] = models.map((model) => ({
				value: model.value,
				label: model.displayName,
				description: model.description
			}));
			delete this.modelStatus[harness];
		} catch {
			this.modelStatus[harness] = 'error';
		}
	}

	selectAgent(agent: string) {
		this.chosenAgent = agent;
	}

	selectModel(model: string) {
		this.chosenModel = model;
	}

	selectTab(id: string) {
		this.activeId = id;
	}

	newSession(): ChatTab | undefined {
		if (!this.harness) return undefined;
		this.tabs.push({ id: `chat-${++tabCounter}`, harness: this.harness, title: '', items: [], running: false });
		const tab = this.tabs[this.tabs.length - 1];
		this.activeId = tab.id;
		return tab;
	}

	/** Closes a session; one that is streaming stops its reply. The harness keeps it. */
	closeTab(id: string) {
		this.#streams.get(id)?.abort();
		this.tabs = this.tabs.filter((tab) => tab.id !== id);
	}

	async send(text: string) {
		const agent = this.agent;
		const harness = this.harness;
		if (this.preview || !harness || !text) return;
		const tab = this.active ?? this.newSession();
		if (!tab || tab.running) return;
		// Send a model only when it comes from this harness's own list.
		const model = this.modelChoices.some((choice) => choice.value === this.model) ? this.model : undefined;
		if (!tab.sessionId && !tab.items.length) tab.title = text.slice(0, 40);
		tab.items.push({ kind: 'user', id: `user-${Date.now()}`, text });
		tab.running = true;
		const controller = new AbortController();
		this.#streams.set(tab.id, controller);
		try {
			await streamHarnessTurn(
				harness,
				{ session_id: tab.sessionId, message: text, model },
				(event) => {
					if (event.type === 'session') tab.sessionId = event.sessionId;
					tab.items = applyEvent(tab.items, event);
				},
				controller.signal
			);
		} catch (error) {
			if (!controller.signal.aborted) this.#notice(tab, errorText(agent, error));
		} finally {
			this.#streams.delete(tab.id);
			tab.running = false;
		}
	}

	async stop() {
		const tab = this.active;
		if (!tab?.sessionId) return;
		try {
			await interruptHarnessTurn(tab.harness, tab.sessionId);
		} catch (error) {
			this.#notice(tab, errorText(this.agent, error));
		}
	}

	async decide(requestId: string, decision: HarnessDecision) {
		const tab = this.active;
		if (!tab?.sessionId) return;
		try {
			await decideHarnessPermission(tab.harness, tab.sessionId, requestId, decision);
			tab.items = withDecision(tab.items, requestId, decision.behavior === 'allow');
		} catch (error) {
			this.#notice(tab, errorText(this.agent, error));
		}
	}

	async loadHistory() {
		if (this.preview || !this.harness) return;
		try {
			this.history = await listHarnessSessions(this.harness);
		} catch {
			this.history = [];
		}
	}

	/** Opens a session the harness keeps, or switches to it if it is open. */
	async openHistory(entry: HarnessSession) {
		const open = this.visible.find((tab) => tab.sessionId === entry.sessionId);
		if (open) {
			this.activeId = open.id;
			return;
		}
		const tab = this.newSession();
		if (!tab) return;
		tab.sessionId = entry.sessionId;
		tab.title = entry.title || entry.firstPrompt || '';
		try {
			tab.items = itemsFromHistory(await getHarnessMessages(tab.harness, entry.sessionId));
		} catch (error) {
			this.#notice(tab, errorText(this.agent, error));
		}
	}

	#notice(tab: ChatTab, detail: string) {
		tab.items.push({ kind: 'notice', id: `notice-${Date.now()}`, code: 'error', detail });
	}

	/** Stops every streaming reply; the page instance is closing. */
	dispose() {
		for (const controller of this.#streams.values()) controller.abort();
		this.#streams.clear();
	}
}

// One chat per page instance of a runtime, kept while any of its chat elements
// is mounted. Hidden pages stay mounted, so hiding keeps the chat.
const chats = new WeakMap<object, Map<string, { chat: ChatSession; users: number }>>();

export function acquireChat(runtime: object, instanceId: string, preview: boolean): ChatSession {
	let byPage = chats.get(runtime);
	if (!byPage) chats.set(runtime, (byPage = new Map()));
	let entry = byPage.get(instanceId);
	if (!entry) byPage.set(instanceId, (entry = { chat: new ChatSession(preview), users: 0 }));
	entry.users++;
	return entry.chat;
}

export function releaseChat(runtime: object, instanceId: string) {
	const byPage = chats.get(runtime);
	const entry = byPage?.get(instanceId);
	if (!byPage || !entry || --entry.users > 0) return;
	entry.chat.dispose();
	byPage.delete(instanceId);
}

// The running WPI app: panes, panels, input docs, menu bindings and the
// unsaved-changes prompt (design D8, D11). Panel bookkeeping decisions come
// from panels.ts; this class keeps the state in $state and talks to the API.
import { getContext, setContext } from 'svelte';
import { runAction, WpiError } from './api';
import { canonical } from './dirty';
import type { Origin } from './entries';
import { planOpen, removePanel, sharingKey, showPanel, type Bindings, type PaneModel, type PanelModel } from './panels';
import { getPath, parseRef, resolveArgs, setPath } from './refs';
import type { ActionRef, Bundle, InputSpec, Json, JsonObject, Trigger } from './types';

export type PanelState = PanelModel & {
	doc: JsonObject;
	// Canonical JSON of the doc as last loaded or saved.
	snapshot: string;
	// Resolved input: {} or a load action with argument values.
	input: InputSpec;
	// False until the first input load has finished.
	ready: boolean;
	loading: boolean;
	error: string | null;
	fieldErrors: Record<string, string[]>;
	// Incremented each time the panel goes from hidden to visible.
	shown: number;
};

export type Choice = 'save' | 'discard' | 'keep';

export type TriggerContext = { panelId: string; record?: JsonObject; binding?: string };

const KEY = Symbol('wpi-runtime');

export function setRuntime(rt: AppRuntime): void {
	setContext(KEY, rt);
}

export function getRuntime(): AppRuntime {
	return getContext<AppRuntime>(KEY);
}

function bindingPath(binding: string | undefined): string[] | null {
	const ref = binding ? parseRef(binding) : null;
	return ref?.root === 'input_doc' ? ref.path : null;
}

function message(err: unknown): string {
	return err instanceof Error ? err.message : String(err);
}

export class AppRuntime {
	readonly bundle: Bundle;
	panes = $state<Record<string, PaneModel>>({});
	panels = $state<Record<string, PanelState>>({});
	bindings = $state<Bindings>({});
	prompt = $state<{ panelId: string; canSave: boolean; resolve: (c: Choice) => void } | null>(null);
	#seq = 0;

	constructor(bundle: Bundle) {
		this.bundle = bundle;
		for (const paneId of Object.keys(bundle.app.panes)) this.panes[paneId] = { panels: [], visible: null };
	}

	// Opens the pages the app descriptor names for its panes.
	start(): void {
		for (const [paneId, spec] of Object.entries(this.bundle.app.panes)) {
			if (spec.page) void this.openPage({ page: spec.page, pane: paneId });
		}
	}

	async openPage(req: { page: string; pane: string; input?: InputSpec; binding?: string }): Promise<void> {
		const pane = this.panes[req.pane];
		const page = this.bundle.pages[req.page];
		if (!pane || !page) throw new Error(`cannot open page "${req.page}" in pane "${req.pane}"`);
		const input = req.input ?? page.input ?? {};
		const key = sharingKey(req.page, input);
		const plan = planOpen(pane, this.panels, this.bindings, { key, binding: req.binding });
		if (plan.current?.fate === 'close' && !(await this.closePanel(plan.current.id, false))) return;
		let id = plan.reuse;
		if (!id) {
			id = `${req.page}#${++this.#seq}`;
			this.panels[id] = {
				id,
				paneId: req.pane,
				pageId: req.page,
				key,
				doc: {},
				snapshot: canonical({}),
				input,
				ready: !input.action,
				loading: false,
				error: null,
				fieldErrors: {},
				shown: 0
			};
		}
		if (req.binding) this.bindings[req.binding] = id;
		this.#show(req.pane, id);
		if (!plan.reuse && input.action) await this.#loadInput(id);
	}

	#show(paneId: string, id: string): void {
		const pane = this.panes[paneId];
		const wasHidden = pane.panels.includes(id) && pane.visible !== id;
		showPanel(pane, id);
		if (!wasHidden) return;
		const panel = this.panels[id];
		panel.shown++;
		if (this.bundle.pages[panel.pageId].refresh?.on_show && panel.input.action && !this.isDirty(id)) {
			void this.#loadInput(id);
		}
	}

	async #loadInput(id: string): Promise<void> {
		const panel = this.panels[id];
		if (!panel?.input.action) return;
		panel.loading = true;
		panel.error = null;
		try {
			const res = await runAction({ kind: 'action', id: panel.input.action }, panel.input.args ?? {});
			if (!this.panels[id]) return;
			panel.doc = (res.result ?? {}) as JsonObject;
			panel.snapshot = canonical(panel.doc);
		} catch (err) {
			panel.error = message(err);
		} finally {
			panel.loading = false;
			panel.ready = true;
		}
	}

	isDirty(id: string): boolean {
		const panel = this.panels[id];
		return !!panel && canonical(panel.doc) !== panel.snapshot;
	}

	anyDirty(): boolean {
		return Object.keys(this.panels).some((id) => this.isDirty(id));
	}

	// Closes a panel after the unsaved-changes check; false if the user kept
	// editing or saving failed. showNext makes the pane show its most recently
	// shown remaining panel.
	async closePanel(id: string, showNext = true): Promise<boolean> {
		const panel = this.panels[id];
		if (!panel) return true;
		if (this.isDirty(id)) {
			const choice = await this.#ask(id);
			if (choice === 'keep') return false;
			if (choice === 'save' && !(await this.save(id))) return false;
		}
		const pane = this.panes[panel.paneId];
		const next = removePanel(pane, this.bindings, id);
		delete this.panels[id];
		if (next) {
			pane.visible = null;
			if (showNext) this.#show(panel.paneId, next);
		}
		return true;
	}

	#ask(panelId: string): Promise<Choice> {
		const canSave = !!this.bundle.pages[this.panels[panelId].pageId].save_action;
		return new Promise((resolve) => {
			this.prompt = {
				panelId,
				canSave,
				resolve: (choice) => {
					this.prompt = null;
					resolve(choice);
				}
			};
		});
	}

	// Saves a panel's input doc with ref, or with its page's save_action.
	async save(id: string, ref?: ActionRef): Promise<boolean> {
		const panel = this.panels[id];
		const saveAction = this.bundle.pages[panel.pageId].save_action;
		const actionRef = ref ?? (saveAction ? ({ kind: 'action', id: saveAction } as const) : null);
		if (!actionRef) return false;
		panel.error = null;
		try {
			const res = await runAction(actionRef, {}, $state.snapshot(panel.doc) as JsonObject);
			setPath(panel.doc, [String(res.key_attribute)], res.key_value);
			panel.snapshot = canonical(panel.doc);
			panel.fieldErrors = {};
			return true;
		} catch (err) {
			if (err instanceof WpiError && err.fieldErrors) panel.fieldErrors = err.fieldErrors;
			else panel.error = message(err);
			return false;
		}
	}

	// Runs an element's trigger. origin locates the trigger in its stored
	// descriptor, for inline actions the server runs.
	async runTrigger(t: Trigger, origin: Origin & { pointer: string }, ctx: TriggerContext): Promise<void> {
		const panel = this.panels[ctx.panelId];
		const args = resolveArgs(t.args, { input_doc: panel?.doc, record: ctx.record });
		const action = t.action ? this.bundle.actions[t.action] : t;
		const ref: ActionRef = t.action ? { kind: 'action', id: t.action } : { kind: origin.kind, id: origin.id, pointer: origin.pointer };
		switch (action.action_type) {
			case 'open_page': {
				const input = action.input?.action ? { action: action.input.action, args: resolveArgs(action.input.args, { args }) } : action.input;
				await this.openPage({ page: action.page!, pane: action.pane!, input, binding: ctx.binding });
				return;
			}
			case 'close_page':
				await this.closePanel(ctx.panelId);
				return;
			case 'save':
				await this.save(ctx.panelId, ref);
				return;
		}
	}

	// Runs a list's source query in the context of its panel.
	async queryList(panelId: string, source: { action: string; args?: JsonObject }): Promise<JsonObject[]> {
		const args = resolveArgs(source.args, { input_doc: this.panels[panelId]?.doc });
		const res = await runAction({ kind: 'action', id: source.action }, args);
		return (res.result ?? []) as JsonObject[];
	}

	read(panelId: string, binding: string | undefined): Json | undefined {
		const path = bindingPath(binding);
		return path ? getPath(this.panels[panelId]?.doc, path) : undefined;
	}

	// Writes an edit to the input doc and clears that attribute's save errors.
	write(panelId: string, binding: string | undefined, value: Json): void {
		const panel = this.panels[panelId];
		const path = bindingPath(binding);
		if (!panel || !path) return;
		setPath(panel.doc, path, value);
		const attr = path.join('.');
		if (panel.fieldErrors[attr]) delete panel.fieldErrors[attr];
	}

	errorsFor(panelId: string, binding: string | undefined): string[] {
		const path = bindingPath(binding);
		return (path && this.panels[panelId]?.fieldErrors[path.join('.')]) || [];
	}

	// The panel a menu binding points at, if it is still open and visible.
	isShowing(binding: string): boolean {
		const id = this.bindings[binding];
		return !!id && Object.values(this.panes).some((p) => p.visible === id);
	}
}

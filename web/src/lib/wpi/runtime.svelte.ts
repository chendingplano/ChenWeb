// The running WPI app: page instances, the panes they own, the Panel Cache, input
// docs and the unsaved-changes prompt (devdoc 2026102601 §3.3; OpenSpec
// wpi-pages-own-panes, design D8, D9). Cache decisions come from panel-cache.ts;
// this class keeps the state in $state and talks to the API.
import { getContext, setContext } from 'svelte';
import { previewList, runAction, WpiError } from './api';
import { previewLoadSource } from './list-request';
import { canonical } from './dirty';
import { isAction, mergeAction, staticInput } from './entries';
import { effectiveRoot, leafPanes } from './layout';
import { findCached, forget, paneInstanceId, paneOwner, sharingKey, show, type PaneModel } from './panel-cache';
import { getPath, parseRef, resolveArgs, setPath } from './refs';
import type { ActionInstance, ActionRef, Bundle, Input, InputSpec, Json, JsonObject } from './types';

// One open copy of a page. hostPane is the pane instance it was opened in, or
// null for the page in the app's page area.
export type PageInstance = {
	id: string;
	pageId: string;
	hostPane: string | null;
	// Instances of the same page with the same input share it (panel-cache.ts).
	key: string;
	doc: JsonObject;
	// Canonical JSON of the doc as last loaded or saved.
	snapshot: string;
	// Resolved input: {}, static data, or an action instance with argument values.
	input: InputSpec;
	// False until the first input load has finished.
	ready: boolean;
	loading: boolean;
	error: string | null;
	fieldErrors: Record<string, string[]>;
	// Incremented each time the instance goes from hidden to shown.
	shown: number;
};

export type Choice = 'save' | 'discard' | 'keep';

export type TriggerContext = { instanceId: string; record?: JsonObject };

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
	pages = $state<Record<string, PageInstance>>({});
	panes = $state<Record<string, PaneModel>>({});
	// Pixel sizes of resized panes, by pane instance.
	paneSizes = $state<Record<string, number>>({});
	// The page instance in the app's page area.
	root = $state<string | null>(null);
	prompt = $state<{ instanceId: string; canSave: boolean; resolve: (c: Choice) => void } | null>(null);
	#seq = 0;

	constructor(bundle: Bundle, readonly editorPreview = false) {
		this.bundle = bundle;
	}

	// Opens the app's initial page in its page area.
	start(): void {
		this.root = this.#create(this.bundle.app.initial_page, null, this.#pageInput(this.bundle.app.initial_page));
	}

	pane(id: string): PaneModel {
		return (this.panes[id] ??= { opened: [], visible: null });
	}

	#create(pageId: string, hostPane: string | null, input: InputSpec): string {
		if (!this.bundle.pages[pageId]) throw new Error(`page "${pageId}" is not in the bundle`);
		const id = `${pageId}#${++this.#seq}`;
		const doc = $state.snapshot<unknown>(input.doc ?? {}) as JsonObject;
		this.pages[id] = {
			id,
			pageId,
			hostPane,
			key: sharingKey(pageId, input),
			doc,
			snapshot: canonical(doc),
			input,
			ready: !input.ref,
			loading: false,
			error: null,
			fieldErrors: {},
			shown: 0
		};
		if (input.ref) void this.#loadInput(id);
		return id;
	}

	// The pane instance a pane ID names, looked up outward from a page instance:
	// its own leaf panes first, then those of the page it was opened in, and so
	// on. An instance opened in a pane has that pane in place of its root, so
	// its root's ID names that pane (devdoc §5.1). null if no page has the pane.
	resolvePane(fromInstance: string, paneId: string): string | null {
		let inst: PageInstance | undefined = this.pages[fromInstance];
		while (inst) {
			const root = effectiveRoot(this.bundle.pages[inst.pageId]);
			if (inst.hostPane && root.pane_id === paneId) return inst.hostPane;
			if (leafPanes(root).includes(paneId)) return paneInstanceId(inst.id, paneId);
			if (!inst.hostPane) return null;
			inst = this.pages[paneOwner(inst.hostPane)];
		}
		return null;
	}

	// Opens a page in a pane: the pane hides what it shows, then shows the
	// cached instance of the page with this input, or a new one (design D8).
	async openPage(req: { page: string; pane: string; from: string; input?: InputSpec }): Promise<void> {
		const paneId = this.resolvePane(req.from, req.pane);
		if (!paneId) throw new Error(`pane "${req.pane}" is not a pane of this page or of a page it is opened in`);
		const input = req.input ?? this.#pageInput(req.page);
		const pane = this.pane(paneId);
		const id = findCached(pane, this.pages, sharingKey(req.page, input)) ?? this.#create(req.page, paneId, input);
		this.#show(paneId, id);
	}

	// A page's own input: static data or its action instance, whose args are
	// literals.
	#pageInput(pageId: string): InputSpec {
		const input = this.bundle.pages[pageId]?.input;
		if (!input) return {};
		if (!isAction(input)) return staticInput(input);
		return { ref: { kind: 'page', id: pageId, pointer: '/input' }, args: resolveArgs(input.args, {}) };
	}

	// An open-page's input: static data or its nested action instance, which
	// receives the open-page's arguments, or its own args resolved against them
	// (design D7).
	#nestedInput(input: Input | undefined, ref: ActionRef, args: JsonObject): InputSpec | undefined {
		if (!input) return undefined;
		if (!isAction(input)) return staticInput(input);
		return { ref: { ...ref, pointer: `${ref.pointer}/input` }, args: input.args ? resolveArgs(input.args, { args }) : args };
	}

	#show(paneId: string, id: string): void {
		const pane = this.pane(paneId);
		const wasHidden = pane.opened.includes(id) && pane.visible !== id;
		show(pane, id);
		if (!wasHidden) return;
		const inst = this.pages[id];
		inst.shown++;
		if (this.bundle.pages[inst.pageId].refresh?.on_show && inst.input.ref && !this.isDirty(id)) {
			void this.#loadInput(id);
		}
	}

	async #loadInput(id: string): Promise<void> {
		const inst = this.pages[id];
		if (!inst?.input.ref) return;
		inst.loading = true;
		inst.error = null;
		try {
			const res = await runAction(inst.input.ref, inst.input.args ?? {});
			if (!this.pages[id]) return;
			inst.doc = (res.result ?? {}) as JsonObject;
			inst.snapshot = canonical(inst.doc);
		} catch (err) {
			inst.error = message(err);
		} finally {
			inst.loading = false;
			inst.ready = true;
		}
	}

	isDirty(id: string): boolean {
		const inst = this.pages[id];
		return !!inst && canonical(inst.doc) !== inst.snapshot;
	}

	anyDirty(): boolean {
		return Object.keys(this.pages).some((id) => this.isDirty(id));
	}

	// Closes a page instance: first the instances opened in its panes, then it,
	// each after the unsaved-changes check; false if the user kept editing or a
	// save failed. Its pane then shows its own panel (design D8).
	async closePage(id: string): Promise<boolean> {
		const inst = this.pages[id];
		if (!inst) return true;
		for (const nested of Object.values(this.pages).filter((p) => p.hostPane?.startsWith(`${id}/`))) {
			if (!(await this.closePage(nested.id))) return false;
		}
		if (this.isDirty(id)) {
			const choice = await this.#ask(id);
			if (choice === 'keep') return false;
			if (choice === 'save' && !(await this.save(id))) return false;
		}
		if (inst.hostPane) forget(this.pane(inst.hostPane), id);
		for (const paneId of Object.keys(this.panes)) {
			if (paneId.startsWith(`${id}/`)) delete this.panes[paneId];
		}
		delete this.pages[id];
		return true;
	}

	#ask(instanceId: string): Promise<Choice> {
		const canSave = !!this.bundle.pages[this.pages[instanceId].pageId].save_action;
		return new Promise((resolve) => {
			this.prompt = {
				instanceId,
				canSave,
				resolve: (choice) => {
					this.prompt = null;
					resolve(choice);
				}
			};
		});
	}

	// Saves a page instance's input doc with ref, or with its page's save_action.
	async save(id: string, ref?: ActionRef): Promise<boolean> {
		const inst = this.pages[id];
		const saveAction = this.bundle.pages[inst.pageId].save_action;
		const actionRef = ref ?? (saveAction ? ({ kind: 'page', id: inst.pageId, pointer: '/save_action' } as const) : null);
		if (!actionRef) return false;
		inst.error = null;
		try {
			const res = await runAction(actionRef, {}, $state.snapshot<unknown>(inst.doc) as JsonObject);
			setPath(inst.doc, [String(res.key_attribute)], res.key_value);
			inst.snapshot = canonical(inst.doc);
			inst.fieldErrors = {};
			return true;
		} catch (err) {
			if (err instanceof WpiError && err.fieldErrors) inst.fieldErrors = err.fieldErrors;
			else inst.error = message(err);
			return false;
		}
	}

	// The page, pane and input an open-page instance asks for, from ctx.
	#openRequest(t: ActionInstance, ref: ActionRef, ctx: TriggerContext) {
		const a = mergeAction(t);
		const args = resolveArgs(a.args, { input_doc: this.pages[ctx.instanceId]?.doc, record: ctx.record });
		return { page: a.page!, pane: a.pane!, from: ctx.instanceId, input: this.#nestedInput(a.input, ref, args) };
	}

	// Runs an action instance an element triggers; ref names it to the server.
	async runTrigger(t: ActionInstance, ref: ActionRef, ctx: TriggerContext): Promise<void> {
		const inst = this.pages[ctx.instanceId];
		try {
			switch (t.action) {
				case 'open-page':
					await this.openPage(this.#openRequest(t, ref, ctx));
					return;
				case 'close-page':
					// The page in the app's page area cannot be closed.
					if (inst?.hostPane) await this.closePage(ctx.instanceId);
					return;
				case 'save':
					await this.save(ctx.instanceId, ref);
					return;
				case 'save-page':
					await this.save(ctx.instanceId);
					return;
			}
		} catch (err) {
			if (inst) inst.error = message(err);
		}
	}

	// Whether an open-page instance's page, with its input, is what its pane
	// shows: a menu item shows itself as current then.
	isShowing(t: ActionInstance, ref: ActionRef, ctx: TriggerContext): boolean {
		if (t.action !== 'open-page') return false;
		const req = this.#openRequest(t, ref, ctx);
		const paneId = this.resolvePane(req.from, req.pane);
		const visible = paneId ? this.panes[paneId]?.visible : null;
		return !!visible && this.pages[visible]?.key === sharingKey(req.page, req.input ?? this.#pageInput(req.page));
	}

	// Runs a list's source load instance in the context of its page instance.
	async queryList(instanceId: string, ref: ActionRef, args?: JsonObject, signal?: AbortSignal, source?: ActionInstance): Promise<JsonObject[]> {
		const resolved = resolveArgs(args, { input_doc: this.pages[instanceId]?.doc });
		const previewSource = this.editorPreview ? previewLoadSource(source) : null;
		if (previewSource) return previewList(previewSource, resolved, signal);
		const res = await runAction(ref, resolved, undefined, fetch, signal);
		return (res.result ?? []) as JsonObject[];
	}

	read(instanceId: string, binding: string | undefined): Json | undefined {
		const path = bindingPath(binding);
		return path ? getPath(this.pages[instanceId]?.doc, path) : undefined;
	}

	// Writes an edit to the input doc and clears that attribute's save errors.
	write(instanceId: string, binding: string | undefined, value: Json): void {
		const inst = this.pages[instanceId];
		const path = bindingPath(binding);
		if (!inst || !path) return;
		setPath(inst.doc, path, value);
		const attr = path.join('.');
		if (inst.fieldErrors[attr]) delete inst.fieldErrors[attr];
	}

	errorsFor(instanceId: string, binding: string | undefined): string[] {
		const path = bindingPath(binding);
		return (path && this.pages[instanceId]?.fieldErrors[path.join('.')]) || [];
	}
}

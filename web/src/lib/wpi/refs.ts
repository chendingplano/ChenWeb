// References ("$input_doc.a.b", "$record.id", "$args.id") and path access.
// Rules match server/api/wpi/refs.go.
import type { Json, JsonObject } from './types';

export type Root = 'input_doc' | 'record' | 'args';
export type Ref = { root: Root; path: string[] };

const SEGMENT = /^[A-Za-z_][A-Za-z0-9_]*$/;
const FORBIDDEN = new Set(['__proto__', 'constructor', 'prototype']);
const ROOTS = new Set<string>(['input_doc', 'record', 'args']);

function checkPath(path: string[]): void {
	for (const seg of path) {
		if (FORBIDDEN.has(seg)) throw new Error(`path segment "${seg}" is not allowed`);
		if (!SEGMENT.test(seg)) throw new Error(`invalid path segment "${seg}"`);
	}
}

// Returns the parsed reference, or null when value is not a reference.
// Throws for malformed references.
export function parseRef(value: unknown): Ref | null {
	if (typeof value !== 'string' || !value.startsWith('$')) return null;
	const [root, ...path] = value.slice(1).split('.');
	if (!ROOTS.has(root)) throw new Error(`unknown reference root in "${value}"`);
	if (path.length === 0) throw new Error(`reference "${value}" needs a path`);
	checkPath(path);
	return { root: root as Root, path };
}

export function getPath(doc: JsonObject | undefined, path: string[]): Json | undefined {
	let cur: Json | undefined = doc;
	for (const seg of path) {
		if (cur === null || typeof cur !== 'object' || Array.isArray(cur) || !Object.hasOwn(cur, seg)) return undefined;
		cur = cur[seg];
	}
	return cur;
}

// Writes value at path, creating missing objects on the way.
export function setPath(doc: JsonObject, path: string[], value: Json): void {
	checkPath(path);
	let cur = doc;
	for (const seg of path.slice(0, -1)) {
		const next = Object.hasOwn(cur, seg) ? cur[seg] : undefined;
		if (next === null || typeof next !== 'object' || Array.isArray(next)) cur[seg] = {};
		cur = cur[seg] as JsonObject;
	}
	cur[path[path.length - 1]] = value;
}

export type Scope = Partial<Record<Root, JsonObject>>;

// A reference becomes the value it points to (null when missing); anything
// else is returned as is.
export function resolveValue(value: Json, scope: Scope): Json {
	const ref = parseRef(value);
	if (!ref) return value;
	const root = scope[ref.root];
	if (!root) throw new Error(`$${ref.root} is not available here`);
	return getPath(root, ref.path) ?? null;
}

export function resolveArgs(args: JsonObject | undefined, scope: Scope): JsonObject {
	const out: JsonObject = {};
	for (const [k, v] of Object.entries(args ?? {})) out[k] = resolveValue(v, scope);
	return out;
}

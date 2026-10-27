// The properties panel's Node tab edits the selected node's descriptor (nodeJson
// in wpi-json.ts) attribute by attribute. A value is typed the way the
// Properties tab types one: JSON when it parses, otherwise the text itself.
import type { Node } from '@xyflow/svelte';
import type { JsonObject } from '$lib/wpi/types';
import { ATTRIBUTE_NAME_PATTERN, parseDefault } from './element-class-form';
import { isElementNode } from './wpi-json';

export type NameProblem = 'name' | 'duplicate';

// An element instance names its class with element, so that attribute is
// neither renamed nor removed, and its value is always text.
export function isClassKey(node: Node, name: string): boolean {
	return isElementNode(node) && name === 'element';
}

// Why an attribute cannot be added with this name, or renamed to it.
export function nameProblem(descriptor: JsonObject, name: string): NameProblem | null {
	if (!ATTRIBUTE_NAME_PATTERN.test(name)) return 'name';
	if (Object.hasOwn(descriptor, name)) return 'duplicate';
	return null;
}

// The descriptor after text was typed for an attribute; a new attribute goes last.
export function withValue(node: Node, descriptor: JsonObject, name: string, text: string): JsonObject {
	return { ...descriptor, [name]: isClassKey(node, name) ? text.trim() : parseDefault(text) };
}

// Renamed in its place, so the attributes keep their order.
export function renamed(descriptor: JsonObject, from: string, to: string): JsonObject {
	return Object.fromEntries(Object.entries(descriptor).map(([k, v]) => [k === from ? to : k, v]));
}

export function without(descriptor: JsonObject, name: string): JsonObject {
	const next = { ...descriptor };
	delete next[name];
	return next;
}

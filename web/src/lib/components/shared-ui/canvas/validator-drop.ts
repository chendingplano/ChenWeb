// Dropping a validator from the palette onto an element of the canvas page
// (OpenSpec wpi-primitive-validators, design D9): the element's validators gain
// an instance holding a copy of the class, and the element becomes the selection.
import type { Node } from '@xyflow/svelte';
import type { ElementClassSummary } from '$lib/wpi/api';
import type { JsonObject } from '$lib/wpi/types';
import { classVersion, makeInstanceId, newDynamicInstanceId, nextSeqno } from '$lib/wpi/instance-ids';
import { newValidatorInstance, type ValidatorClass } from '$lib/wpi/validators';

// The code module of an element node: its class copy's, else its listed class's.
export function elementModule(node: Node, classes: ElementClassSummary[]): string {
	const copy = node.data.class as JsonObject | undefined;
	if (typeof copy?.code_module_id === 'string') return copy.code_module_id;
	return classes.find((c) => c.descriptor_id === node.data.element)?.code_module_id ?? '';
}

// `taken` are the page's other descriptors, whose seqnos are in use.
export function addValidator(nodes: Node[], nodeId: string, cls: ValidatorClass, taken: JsonObject[] = []): Node[] {
	return nodes.map((n) => {
		if (n.id !== nodeId) return { ...n, selected: false };
		const attrs = (n.data.attrs as JsonObject | undefined) ?? {};
		const list = Array.isArray(attrs.validators) ? attrs.validators : [];
		const instance = {
			...newValidatorInstance(cls),
			instance_id: makeInstanceId(cls.descriptor_id, classVersion('validator', cls.descriptor_id) ?? 1, nextSeqno([...taken, ...nodes.map((x) => ({ instance_id: x.data.instance_id, attrs: x.data.attrs }) as JsonObject)])),
			dynamic_instance_id: newDynamicInstanceId()
		} as unknown as JsonObject;
		const validators = [...list, instance];
		return { ...n, selected: true, data: { ...n.data, attrs: { ...attrs, validators } } };
	});
}

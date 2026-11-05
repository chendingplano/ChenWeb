import test from 'node:test';
import assert from 'node:assert/strict';
import { makeInstanceId, newDynamicInstanceId, nextSeqno, setClassVersions, stampInstances, stripDynamicIds, stripInstanceIds } from './instance-ids';

test('instance_id is descriptor, version and page-local seqno', () => {
	assert.equal(makeInstanceId('input', 4, 1), 'input_4_1');
});

test('seqno counts over elements and their validators', () => {
	assert.equal(nextSeqno([]), 1);
	assert.equal(nextSeqno([{ instance_id: 'input_4_1' }, { instance_id: 'text_1_7' }]), 8);
	assert.equal(nextSeqno([{ instance_id: 'input_4_1', attrs: { validators: [{ instance_id: 'decimal_2_5' }] } }]), 6);
});

test('dynamic ids are unique and dropped on save', () => {
	assert.notEqual(newDynamicInstanceId(), newDynamicInstanceId());
	assert.deepEqual(stripDynamicIds({ instance_id: 'a_1_1', dynamic_instance_id: 'x' }), { instance_id: 'a_1_1' });
	assert.deepEqual(stripInstanceIds({ a: [{ instance_id: 'a_1_1', dynamic_instance_id: 'x', k: 1 }] }), { a: [{ k: 1 }] });
});

test('seqno also counts action instances at any depth', () => {
	assert.equal(nextSeqno([{ instance_id: 'list_1_1', attrs: { on_row_click: { action: 'open-page', instance_id: 'open-page_2_9' } } }]), 10);
});

test('actions, validators and layouters are numbered when placed, with versions from the registry', () => {
	setClassVersions({ action: { 'save-page': 3 }, validator: { integer: 2 }, 'page-layouter': { 'single-pane': 5 }, 'element-layouter': { 'flow-layouter': 7 } });
	const page = {
		page_layouter: { layouter: 'single-pane', class: { descriptor_id: 'single-pane' } },
		panels: { main: { elem_layouter: { layouter: 'flow-layouter' }, elem_list: [{ element: 'button', on_click: { action: 'save-page' } }] } },
		save_action: { action: 'save-page' }
	};
	const out = stampInstances(page, [{ instance_id: 'input_4_9' }]) as any;
	assert.equal(out.page_layouter.instance_id, 'single-pane_5_10');
	assert.equal(out.panels.main.elem_layouter.instance_id, 'flow-layouter_7_11');
	assert.equal(out.save_action.instance_id, 'save-page_3_12');
	assert.equal(out.page_layouter.class.instance_id, undefined);
	assert.equal(out.panels.main.elem_list[0].on_click.instance_id, undefined, 'the nodes own elem_list');
	for (const c of [out.page_layouter, out.panels.main.elem_layouter, out.save_action]) assert.ok(c.dynamic_instance_id);
	// idempotent: nothing is renumbered
	assert.deepEqual(stampInstances(out, [out]), out);
	const attrs = stampInstances({ on_click: { action: 'save-page' }, validators: [{ validator: 'integer' }] }, []) as any;
	assert.equal(attrs.on_click.instance_id, 'save-page_3_1');
	assert.equal(attrs.validators[0].instance_id, 'integer_2_2');
});

test('without taken, only dynamic ids are added, copies left alone', () => {
	const attrs = { on_click: { action: 'save-page', class: { descriptor_id: 'save-page' } }, validators: [{ validator: 'integer' }] };
	const stamped = stampInstances(attrs, null) as any;
	assert.ok(stamped.on_click.dynamic_instance_id && stamped.validators[0].dynamic_instance_id);
	assert.equal(stamped.on_click.instance_id, undefined);
	assert.equal(stamped.on_click.class.dynamic_instance_id, undefined);
	assert.equal(stampInstances(stamped, null).on_click.dynamic_instance_id, stamped.on_click.dynamic_instance_id);
	assert.deepEqual(stripDynamicIds(stamped), attrs);
});

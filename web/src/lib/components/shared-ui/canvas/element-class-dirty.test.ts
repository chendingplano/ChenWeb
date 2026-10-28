import test from 'node:test';
import assert from 'node:assert/strict';
import { classRowKey } from './element-class-dirty';
import { attributesFromRow, buildClassRow, newAttribute } from './element-class-form';

const row = {
	descriptor_id: 'button',
	schema_version: '0.4',
	label: { default_value: 'Run', help_tip: { en: 'Caption', 'zh-cn': '标题' } }
};

test('loading or rebuilding an unchanged descriptor stays clean', () => {
	const attributes = attributesFromRow(row);
	assert.equal(classRowKey(attributes), classRowKey(attributesFromRow(buildClassRow(attributes))));
	assert.equal(classRowKey(attributes), classRowKey(attributesFromRow({
		label: { help_tip: { 'zh-cn': '标题', en: 'Caption' }, default_value: 'Run' },
		schema_version: '0.4', descriptor_id: 'button'
	})), 'JSON object key order does not mark a descriptor dirty');
});

test('changes enable saving and restoring the original descriptor makes it clean', () => {
	const original = attributesFromRow(row);
	const baseline = classRowKey(original);
	const edited = attributesFromRow({ ...row, label: { ...row.label, default_value: 'Go' } });
	assert.notEqual(classRowKey(edited), baseline);
	assert.notEqual(classRowKey([...original, newAttribute('width', false, '200px', '', '')]), baseline);
	assert.notEqual(classRowKey(original.filter((a) => a.name !== 'label')), baseline);
	assert.equal(classRowKey(attributesFromRow(row)), baseline);
});

test('array order and null versus missing defaults remain meaningful changes', () => {
	assert.notEqual(classRowKey(attributesFromRow({ items: { default_value: [1, 2] } })), classRowKey(attributesFromRow({ items: { default_value: [2, 1] } })));
	assert.notEqual(classRowKey(attributesFromRow({ label: {} })), classRowKey(attributesFromRow({ label: { default_value: null } })));
});

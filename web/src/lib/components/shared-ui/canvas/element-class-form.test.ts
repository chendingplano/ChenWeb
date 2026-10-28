import test from 'node:test';
import assert from 'node:assert/strict';
import {
	applyEdit,
	attributeFields,
	attributeFromFields,
	attributesFromRow,
	attributeProblem,
	buildClassRow,
	CLASS_ID_PATTERN,
	defaultValue,
	editText,
	hasDefault,
	helpTip,
	initialAttributes,
	isClassAttribute,
	isFixed,
	newAttribute,
	parseDefault,
	renameProblem,
	rowProblem
} from './element-class-form';

test('the form starts with the class attributes', () => {
	assert.deepEqual(initialAttributes(), [
		{ name: 'descriptor_id', value: '' },
		{ name: 'schema_version', value: '0.4' },
		{ name: 'code_module_id', value: '' },
		{ name: 'description', value: { en: '', 'zh-cn': '' } },
		{ name: 'implement_notes', value: '' }
	]);
	assert.ok(isClassAttribute('implement_notes'));
	assert.ok(isClassAttribute('code_module_id'));
	assert.ok(!isClassAttribute('label'));
});

test('a default is JSON when it parses, otherwise text', () => {
	assert.equal(parseDefault('100'), 100);
	assert.equal(parseDefault('true'), true);
	assert.deepEqual(parseDefault('{"en": "Name"}'), { en: 'Name' });
	assert.equal(parseDefault('"123"'), '123');
	assert.equal(parseDefault(' 200px '), '200px');
	assert.equal(parseDefault('read-write'), 'read-write');
});

test('a new instance attribute is an object with a help tip and a default', () => {
	assert.deepEqual(newAttribute('label', true, 'ignored', 'Caption', '标题'), { name: 'label', value: { help_tip: { en: 'Caption', 'zh-cn': '标题' }, default_value: null } });
	assert.deepEqual(newAttribute('max', false, '100', 'Largest value', ''), { name: 'max', value: { help_tip: { en: 'Largest value' }, default_value: 100 } });
	assert.deepEqual(newAttribute('on_change', false, '  ', '', ''), { name: 'on_change', value: {} }, 'no default: optional');
	const required = newAttribute('label', true, '', '', '');
	assert.ok(hasDefault(required));
	assert.equal(defaultValue(required), null);
	assert.ok(!hasDefault(newAttribute('x', false, '', '', '')));
});

test('attribute problems', () => {
	const added = [...initialAttributes(), newAttribute('label', true, '', '', '')];
	assert.equal(attributeProblem('max-value', '', '', added), 'name');
	assert.equal(attributeProblem('', '', '', added), 'name');
	assert.equal(attributeProblem('element', '', '', added), 'reserved');
	assert.equal(attributeProblem('implement_notes', '', '', added), 'reserved');
	assert.equal(attributeProblem('label', '', '', added), 'duplicate');
	assert.equal(attributeProblem('max', '', '最大值', added), 'help_tip_en');
	assert.equal(attributeProblem('max', 'Largest', '最大值', added), null);
});

test('renaming an instance attribute', () => {
	const attrs = [...initialAttributes(), newAttribute('label', true, '', '', ''), newAttribute('max', false, '100', '', '')];
	assert.equal(renameProblem('max', 'max', attrs), null);
	assert.equal(renameProblem('max', 'maximum', attrs), null);
	assert.equal(renameProblem('max', 'label', attrs), 'duplicate');
	assert.equal(renameProblem('max', 'description', attrs), 'reserved');
	assert.equal(renameProblem('max', 'max-value', attrs), 'name');
});

test('editing class attributes: ID, version, code module and notes are text, the description JSON or text', () => {
	const [id, version, module, description, notes] = initialAttributes();
	assert.deepEqual(applyEdit(id, 'value', ' 123 '), { name: 'descriptor_id', value: '123' });
	assert.deepEqual(applyEdit(version, 'value', '0.4'), { name: 'schema_version', value: '0.4' });
	assert.deepEqual(applyEdit(module, 'value', ' button '), { name: 'code_module_id', value: 'button' });
	assert.deepEqual(applyEdit(notes, 'value', 'Render {a range}\n'), { name: 'implement_notes', value: 'Render {a range}' });
	assert.deepEqual(applyEdit(description, 'value', '{"en": "A", "zh-cn": "甲"}').value, { en: 'A', 'zh-cn': '甲' });
	assert.equal(applyEdit(description, 'value', "The element for 'input'").value, "The element for 'input'");
	for (const attr of initialAttributes()) {
		assert.deepEqual(applyEdit(attr, 'value', editText(attr, 'value')), attr, attr.name);
	}
});

test('editing an instance attribute: default and help tip', () => {
	const max = newAttribute('max', false, '100', 'Largest', '');
	assert.equal(editText(max, 'value'), '100');
	assert.equal(editText(max, 'help_tip'), '{"en":"Largest"}');
	assert.deepEqual(applyEdit(max, 'value', '50').value, { help_tip: { en: 'Largest' }, default_value: 50 });
	assert.deepEqual(applyEdit(max, 'value', '').value, { help_tip: { en: 'Largest' } }, 'empty: no default');
	assert.deepEqual(applyEdit(max, 'value', 'null').value, { help_tip: { en: 'Largest' }, default_value: null });
	assert.deepEqual(applyEdit(max, 'help_tip', '').value, { default_value: 100 });
	assert.deepEqual(applyEdit(max, 'help_tip', 'Plain tip').value, { help_tip: 'Plain tip', default_value: 100 });
	const optional = newAttribute('on_change', false, '', '', '');
	assert.equal(editText(optional, 'value'), '');
	assert.deepEqual(applyEdit(optional, 'help_tip', '{"en": "Runs on change"}').value, { help_tip: { en: 'Runs on change' } });
	for (const attr of [max, optional, newAttribute('label', true, '', 'Caption', '标题'), newAttribute('w', false, '"240px"', '', '')]) {
		for (const field of ['value', 'help_tip'] as const) {
			assert.deepEqual(applyEdit(attr, field, editText(attr, field)), attr, `${attr.name} ${field}`);
		}
	}
	assert.deepEqual(helpTip(newAttribute('label', true, '', 'Caption', '')), { en: 'Caption' });
});

test('class IDs', () => {
	assert.ok(CLASS_ID_PATTERN.test('slider'));
	assert.ok(CLASS_ID_PATTERN.test('date-range-2'));
	assert.ok(!CLASS_ID_PATTERN.test('Slider'));
	assert.ok(!CLASS_ID_PATTERN.test('date_range'));
});

test('the class row keeps the attributes in order, drops empty description texts and empty notes', () => {
	const attrs = initialAttributes();
	attrs[0].value = 'slider';
	attrs[3].value = { en: 'A slider', 'zh-cn': '' };
	attrs.push(newAttribute('label', true, '', 'Caption', ''), newAttribute('max', false, '100', '', ''));
	const row = buildClassRow(attrs);
	assert.deepEqual(row, {
		descriptor_id: 'slider',
		schema_version: '0.4',
		description: { en: 'A slider' },
		label: { help_tip: { en: 'Caption' }, default_value: null },
		max: { default_value: 100 }
	});
	assert.deepEqual(Object.keys(row), ['descriptor_id', 'schema_version', 'description', 'label', 'max'], 'a draft need not name its code module');
	attrs[4].value = 'Render a range input.';
	assert.equal(buildClassRow(attrs).implement_notes, 'Render a range input.');
	attrs[2].value = 'input';
	assert.deepEqual(Object.keys(buildClassRow(attrs)).slice(0, 3), ['descriptor_id', 'schema_version', 'code_module_id']);
});

test('row problems', () => {
	const row = (id: string, description: unknown) => buildClassRow([{ name: 'descriptor_id', value: id }, { name: 'description', value: description as never }]);
	assert.equal(rowProblem(row('slider', { en: 'A slider' })), null);
	assert.equal(rowProblem(row('slider', 'A slider')), null);
	assert.equal(rowProblem(row('', { en: 'A slider' })), 'class_id');
	assert.equal(rowProblem(row('Slider', { en: 'A slider' })), 'class_id');
	assert.equal(rowProblem(row('slider', { en: '', 'zh-cn': '滑块' })), 'description');
	assert.equal(rowProblem(row('slider', '')), 'description');
});

test('a stored row becomes the form attributes, class attributes first', () => {
	const row = {
		label: { help_tip: { en: 'Caption' }, default_value: null },
		description: { en: 'A field' },
		descriptor_id: 'input',
		schema_version: '0.4',
		code_module_id: 'input',
		width: { default_value: 'elastic' }
	};
	const attrs = attributesFromRow(row);
	assert.deepEqual(attrs, [
		{ name: 'descriptor_id', value: 'input' },
		{ name: 'schema_version', value: '0.4' },
		{ name: 'code_module_id', value: 'input' },
		{ name: 'description', value: { en: 'A field', 'zh-cn': '' } },
		{ name: 'implement_notes', value: '' },
		{ name: 'label', value: { help_tip: { en: 'Caption' }, default_value: null } },
		{ name: 'width', value: { default_value: 'elastic' } }
	]);
	// Saved unchanged, it is the same row.
	assert.deepEqual(buildClassRow(attrs), row);
	// A description that is plain text stays text.
	assert.equal(attributesFromRow({ description: 'A field' })[3].value, 'A field');
});

test('an empty description is left out of the row', () => {
	const attrs = attributesFromRow({ descriptor_id: 'text', schema_version: '0.4' });
	assert.deepEqual(buildClassRow(attrs), { descriptor_id: 'text', schema_version: '0.4' });
});

test('attributeFields is the inverse of newAttribute', () => {
	for (const [name, required, def, en, zh, fixed] of [
		['width', false, '200', 'Width', '宽度', false],
		['label', true, '', 'Label', '', false],
		['mode', false, '"read-write"', '', '', false],
		['plain', false, '', '', '', false],
		['style', false, '"fancy"', 'Style', '', true]
	] as const) {
		const attr = newAttribute(name, required, def, en, zh, fixed);
		const fields = attributeFields(attr);
		assert.deepEqual(fields, { name, defaultText: def, tipEn: en, tipZh: zh, required, fixed });
		assert.deepEqual(attributeFromFields(fields), attr);
	}
});

test('class attributes go through the same fields', () => {
	for (const attr of [
		{ name: 'description', value: { en: 'A button', 'zh-cn': '按钮' } },
		{ name: 'implement_notes', value: 'line 1\nline 2' },
		{ name: 'schema_version', value: '0.4' },
		{ name: 'code_module_id', value: 'button' }
	]) {
		assert.deepEqual(attributeFromFields(attributeFields(attr)), attr);
	}
	assert.deepEqual(attributeFields({ name: 'description', value: { en: 'A', 'zh-cn': '甲' } }).tipZh, '甲');
});

test('a fixed attribute carries instance_modifiable false and needs a default', () => {
	const style = newAttribute('style', false, 'fancy', 'Style', '', true);
	assert.deepEqual(style, { name: 'style', value: { help_tip: { en: 'Style' }, default_value: 'fancy', instance_modifiable: false } });
	assert.ok(isFixed(style));
	assert.ok(!isFixed(newAttribute('style', false, 'fancy', '', '')));
	assert.ok(!isFixed(newAttribute('label', true, '', '', '', true)), 'every instance must set it: it cannot be fixed');
	assert.deepEqual(applyEdit(style, 'value', '"plain"').value, { help_tip: { en: 'Style' }, default_value: 'plain', instance_modifiable: false });
	assert.deepEqual(applyEdit(style, 'help_tip', '').value, { default_value: 'fancy', instance_modifiable: false });
	assert.deepEqual(buildClassRow([...initialAttributes(), style]).style, { help_tip: { en: 'Style' }, default_value: 'fancy', instance_modifiable: false });
	assert.equal(attributeProblem('style', '', '', initialAttributes(), true, ''), 'fixed_default');
	assert.equal(attributeProblem('style', '', '', initialAttributes(), true, 'null'), 'fixed_default');
	assert.equal(attributeProblem('style', '', '', initialAttributes(), true, 'fancy'), null);
	assert.equal(attributeProblem('code_module_id', '', '', initialAttributes()), 'reserved');
});

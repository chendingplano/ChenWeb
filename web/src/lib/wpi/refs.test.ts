import test from 'node:test';
import assert from 'node:assert/strict';
import { getPath, parseRef, resolveArgs, resolveValue, setPath } from './refs';
import type { JsonObject } from './types';

test('parseRef recognises references and rejects bad ones', () => {
	assert.equal(parseRef('plain'), null);
	assert.equal(parseRef(7), null);
	assert.deepEqual(parseRef('$input_doc.address.city'), { root: 'input_doc', path: ['address', 'city'] });
	assert.deepEqual(parseRef('$record.id'), { root: 'record', path: ['id'] });
	assert.throws(() => parseRef('$input_doc'), /needs a path/);
	assert.throws(() => parseRef('$state.x'), /unknown reference root/);
	assert.throws(() => parseRef('$input_doc.__proto__.x'), /not allowed/);
	assert.throws(() => parseRef('$input_doc.a-b'), /invalid path segment/);
});

test('setPath grows an empty doc through nested paths', () => {
	const doc: JsonObject = {};
	setPath(doc, ['user_name'], 'dana');
	setPath(doc, ['address', 'city'], 'Paris');
	assert.deepEqual(doc, { user_name: 'dana', address: { city: 'Paris' } });
	assert.equal(getPath(doc, ['address', 'city']), 'Paris');
	assert.equal(getPath(doc, ['address', 'zip']), undefined);
	assert.equal(getPath(doc, ['user_name', 'x']), undefined);
});

test('setPath refuses prototype pollution', () => {
	const doc: JsonObject = {};
	assert.throws(() => setPath(doc, ['__proto__', 'polluted'], true));
	assert.equal(({} as Record<string, unknown>).polluted, undefined);
});

test('getPath ignores inherited properties', () => {
	assert.equal(getPath({}, ['toString']), undefined);
});

test('resolveArgs takes values from the scope', () => {
	const scope = { record: { id: 7, email: 'g@example.com' }, input_doc: { user_name: 'dana' } };
	assert.deepEqual(resolveArgs({ id: '$record.id', who: '$input_doc.user_name', fixed: 3, missing: '$record.nope' }, scope), {
		id: 7,
		who: 'dana',
		fixed: 3,
		missing: null
	});
	assert.deepEqual(resolveArgs(undefined, scope), {});
	assert.throws(() => resolveValue('$args.id', scope), /not available/);
});

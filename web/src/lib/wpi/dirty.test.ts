import test from 'node:test';
import assert from 'node:assert/strict';
import { canonical, isDirty } from './dirty';

test('canonical ignores key order and undefined values', () => {
	assert.equal(canonical({ b: 1, a: { d: [1, { y: 2, x: 1 }], c: undefined } }), '{"a":{"d":[1,{"x":1,"y":2}]},"b":1}');
	assert.equal(canonical(undefined), 'null');
});

test('editing a field and changing it back is not dirty', () => {
	const doc: Record<string, unknown> = { user_name: 'alice', email: 'a@example.com' };
	const snapshot = canonical(doc);
	doc.user_name = 'alicia';
	assert.equal(isDirty(doc, snapshot), true);
	doc.user_name = 'alice';
	assert.equal(isDirty(doc, snapshot), false);
});

test('adding an attribute to an empty doc makes it dirty', () => {
	const doc: Record<string, unknown> = {};
	const snapshot = canonical(doc);
	doc.user_name = 'dana';
	assert.equal(isDirty(doc, snapshot), true);
});

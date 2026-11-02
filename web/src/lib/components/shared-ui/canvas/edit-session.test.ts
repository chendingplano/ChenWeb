import test from 'node:test';
import assert from 'node:assert/strict';
import { EditSession } from './edit-session';

test('typing in one property is one step', () => {
	const s = new EditSession();
	s.start();
	assert.deepEqual([s.takesSnapshot(), s.takesSnapshot(), s.takesSnapshot()], [true, false, false]);
	s.end();
});

test('two properties are two steps', () => {
	const s = new EditSession();
	s.start();
	assert.equal(s.takesSnapshot(), true);
	s.end();
	s.start();
	assert.deepEqual([s.takesSnapshot(), s.takesSnapshot()], [true, false]);
});

test('without focus every change is a step', () => {
	const s = new EditSession();
	assert.deepEqual([s.takesSnapshot(), s.takesSnapshot()], [true, true]);
});

test('a canvas action or undo during a focus starts a new step', () => {
	const s = new EditSession();
	s.start();
	assert.equal(s.takesSnapshot(), true);
	s.interrupt();
	assert.deepEqual([s.takesSnapshot(), s.takesSnapshot()], [true, false]);
});

import test from 'node:test';
import assert from 'node:assert/strict';

import { formatInputStatus } from './kb-import-status-summary.js';

test('lists operations without labels when every entry succeeded', () => {
	assert.equal(formatInputStatus([
		{ operation: 'parsed', proc_status: 'success' },
		{ operation: 'chunking', proc_status: 'success' }
	]), 'parsed, chunking');
});

test('lists failed operations before successful operations', () => {
	assert.equal(formatInputStatus([
		{ operation: 'parsed', proc_status: 'success' },
		{ operation: 'chunking', proc_status: 'failed' },
		{ operation: 'extract_metrics', proc_status: 'fail' },
		{ operation: 'embedding', proc_status: 'success' }
	]), 'Failed: chunking, extract_metrics, Success: parsed, embedding');
});

test('keeps in-progress and duplicated entries distinct from failures', () => {
	assert.equal(formatInputStatus([
		{ operation: 'parsed', proc_status: 'active' },
		{ operation: 'chunking', proc_status: 'success' }
	]), 'Active: parsed, Success: chunking');
	assert.equal(formatInputStatus([{ operation: 'parsed', proc_status: 'duplicated' }]), 'Duplicated: parsed');
});

test('shows stopped and pending operations before successes, including hidden operations', () => {
	assert.equal(formatInputStatus([
		{ operation: 'parsed', proc_status: 'success' },
		{ operation: 'doc_processing', proc_status: 'stopped' },
		{ operation: 'converted', proc_status: 'success' },
		{ operation: 'stop_requested', proc_status: 'pending' }
	]), 'Stopped: doc_processing, Pending: stop_requested, Success: parsed, converted');
});

test('uses a placeholder when there are no operations', () => {
	assert.equal(formatInputStatus([]), '-');
	assert.equal(formatInputStatus([{ proc_status: 'success' }]), '-');
});

test('excludes successful internal operations but shows non-success ones', () => {
	assert.equal(formatInputStatus([
		{ operation: 'doc_processing', proc_status: 'active' },
		{ operation: 'blocking', proc_status: 'failed' },
		{ operation: 'static_analyzer', proc_status: 'success' },
		{ operation: 'static_analzyer', proc_status: 'success' },
		{ operation: 'extract_metadata', proc_status: 'success' },
		{ operation: 'chunked', proc_status: 'success' },
		{ operation: 'parsed', proc_status: 'success' },
		{ operation: 'extract_metrics', proc_status: 'failed' }
	]), 'Failed: blocking, extract_metrics, Active: doc_processing, Success: parsed');
	assert.equal(formatInputStatus([{ operation: 'blocking', proc_status: 'failed' }]), 'Failed: blocking');
	assert.equal(formatInputStatus([
		{ operation: ' STATIC_ANALYZER ', proc_status: 'success' },
		{ operation: ' STATIC_ANALZYER ', proc_status: 'success' },
		{ operation: 'parsed', proc_status: 'success' }
	]), 'parsed');
});

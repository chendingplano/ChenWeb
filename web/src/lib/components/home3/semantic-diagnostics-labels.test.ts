import test from 'node:test';
import assert from 'node:assert/strict';

import {
	stateLabel,
	stateSeverity,
	statusLabel,
	statusSeverity
} from './semantic-diagnostics-labels.js';

test('stateLabel renders known governed terms with a human label', () => {
	assert.equal(stateLabel('semantic:value_state_unparsed'), 'Unparsed');
	assert.equal(stateLabel('semantic:mapping_resolved'), 'Resolved');
	assert.equal(stateLabel('semantic:conformance_contract_violation'), 'Contract violation');
});

test('stateLabel falls back to a prettified form for unrecognized terms', () => {
	assert.equal(stateLabel('semantic:some_new_state'), 'Some New State');
	assert.equal(stateLabel('no_prefix_here'), 'No Prefix Here');
});

test('stateLabel handles missing values', () => {
	assert.equal(stateLabel(undefined), '—');
	assert.equal(stateLabel(null), '—');
	assert.equal(stateLabel(''), '—');
});

test('stateSeverity classifies known states and defaults unknowns to neutral', () => {
	assert.equal(stateSeverity('semantic:value_present'), 'good');
	assert.equal(stateSeverity('semantic:mapping_state_unresolved'), 'bad');
	assert.equal(stateSeverity('semantic:provisional_new'), 'warn');
	assert.equal(stateSeverity('semantic:mapping_not_required'), 'neutral');
	assert.equal(stateSeverity('semantic:unknown_future_term'), 'neutral');
	assert.equal(stateSeverity(undefined), 'neutral');
});

test('statusLabel and statusSeverity cover the assertion lifecycle', () => {
	assert.equal(statusLabel('in_review'), 'In Review');
	assert.equal(statusSeverity('accepted'), 'good');
	assert.equal(statusSeverity('unsupported'), 'bad');
	assert.equal(statusSeverity('represented'), 'neutral');
	assert.equal(statusSeverity('made_up_status'), 'neutral');
});

import test from 'node:test';
import assert from 'node:assert/strict';

import {
	createRailState,
	handleRailButtonAction,
	handleRailHoverChange
} from './rail-state.js';

test('manual mode starts expanded by default', () => {
	const state = createRailState({ autoShrinkExpand: false });

	assert.deepEqual(state, {
		autoShrinkExpand: false,
		railExpanded: true
	});
});

test('manual mode can restore a persisted collapsed state', () => {
	const state = createRailState({
		autoShrinkExpand: false,
		railExpanded: false
	});

	assert.deepEqual(state, {
		autoShrinkExpand: false,
		railExpanded: false
	});
});

test('button disables auto mode and keeps the rail expanded', () => {
	const next = handleRailButtonAction({
		autoShrinkExpand: true,
		railExpanded: false
	});

	assert.deepEqual(next, {
		autoShrinkExpand: false,
		railExpanded: true
	});
});

test('button toggles rail expansion in manual mode', () => {
	const expanded = handleRailButtonAction({
		autoShrinkExpand: false,
		railExpanded: false
	});
	const collapsed = handleRailButtonAction(expanded);

	assert.equal(expanded.railExpanded, true);
	assert.equal(collapsed.railExpanded, false);
	assert.equal(collapsed.autoShrinkExpand, false);
});

test('hover only expands the rail when auto mode is enabled', () => {
	assert.deepEqual(
		handleRailHoverChange(
			{
				autoShrinkExpand: true,
				railExpanded: false
			},
			true
		),
		{
			autoShrinkExpand: true,
			railExpanded: true
		}
	);

	assert.deepEqual(
		handleRailHoverChange(
			{
				autoShrinkExpand: false,
				railExpanded: false
			},
			true
		),
		{
			autoShrinkExpand: false,
			railExpanded: false
		}
	);
});

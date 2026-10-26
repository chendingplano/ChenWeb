// Layout templates by name (devdoc 2026102601 §3.2). Pane IDs per template
// match server/api/wpi/types.go.
import type { Component } from 'svelte';
import SinglePane from './SinglePane.svelte';
import TopLeftMiddleRightBottom from './TopLeftMiddleRightBottom.svelte';

export const templates: Record<string, Component> = {
	'single-pane': SinglePane,
	'top-left-middle-right-bottom': TopLeftMiddleRightBottom
};

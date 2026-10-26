// Layouters by name (devdoc 2026102601 §3.7).
import type { Component } from 'svelte';
import type { PageDescriptor } from '../types';
import FlowLayouter from './FlowLayouter.svelte';

export type LayouterProps = { panelId: string; page: PageDescriptor };

export const layouters: Record<string, Component<LayouterProps>> = {
	'flow-layouter': FlowLayouter
};

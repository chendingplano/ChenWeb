// Element layouters by code module ID (devdoc 2026102601 §3.7): the code a
// panel's element layouter copy names in its class's code_module_id. The
// backend validator lists their parameters under the same IDs
// (elementLayouterModules in server/api/wpi/validate.go).
import type { Component } from 'svelte';
import type { PageDescriptor, Panel } from '../types';
import FlowLayouter from './FlowLayouter.svelte';

export type LayouterProps = { instanceId: string; page: PageDescriptor; paneId: string; panel: Panel };

export const elementLayouters: Record<string, Component<LayouterProps>> = {
	'flow-layouter': FlowLayouter
};

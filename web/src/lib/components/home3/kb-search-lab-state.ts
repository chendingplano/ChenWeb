import type { KbSearchArtifactType } from '$lib/services/kbArtifactSearch';

export const kbSearchArtifactOptions: Array<{ value: KbSearchArtifactType; label: string }> = [
	{ value: 'all', label: 'All Artifacts' },
	{ value: 'metrics', label: 'Metrics' },
	{ value: 'summaries', label: 'Summaries' },
	{ value: 'topics', label: 'Topics' },
	{ value: 'scene-blocks', label: 'Scene Blocks' },
	{ value: 'provisions', label: 'Provisions' },
	{ value: 'products', label: 'Products' }
];

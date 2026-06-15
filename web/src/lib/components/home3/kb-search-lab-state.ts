import type { KbSearchArtifactType } from '$lib/services/kbArtifactSearch';

export const kbSearchArtifactOptions: Array<{ value: KbSearchArtifactType; label: string }> = [
	{ value: 'all', label: 'All Artifacts' },
	{ value: 'metrics', label: 'Metrics' },
	{ value: 'summaries', label: 'Summaries' },
	{ value: 'topics', label: 'Topics' },
	{ value: 'content-segments', label: 'Content Segments' },
	{ value: 'semantic-projections', label: 'Semantic Projections' },
	{ value: 'entities', label: 'Entities' },
	{ value: 'relations', label: 'Relations' },
	{ value: 'scene-blocks', label: 'Scene Blocks' },
	{ value: 'provisions', label: 'Provisions' }
];

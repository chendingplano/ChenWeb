import type { TopicCategoryNodeApi } from '$lib/services/kbService';

export type TopicCategoryPathMetadata = TopicCategoryNodeApi['metadata'];

export type TopicCategorySegmentDisplay = {
	label: string;
	path: string;
	tooltip: string;
};

export type TopicCategoryPathDisplay = {
	path: string;
	segments: TopicCategorySegmentDisplay[];
	confidenceLabel: string;
};

export function buildTopicCategoryMetadataByPath(
	nodes: TopicCategoryNodeApi[]
): Record<string, TopicCategoryPathMetadata> {
	const byPath: Record<string, TopicCategoryPathMetadata> = {};
	for (const node of nodes) {
		const path = node.categoryPath?.trim();
		if (!path) continue;
		byPath[path] = node.metadata;
	}
	return byPath;
}

export function buildTopicCategoryPathDisplay(
	path: string,
	metadataByPath: Record<string, TopicCategoryPathMetadata>
): TopicCategoryPathDisplay {
	const normalizedPath = path.trim();
	const labels = normalizedPath
		.split('/')
		.map((part) => part.trim())
		.filter(Boolean);
	const segments = labels.map((label, index) => {
		const segmentPath = labels.slice(0, index + 1).join('/');
		return {
			label,
			path: segmentPath,
			tooltip: buildTopicCategoryTooltip(label, segmentPath, metadataByPath[segmentPath])
		};
	});
	const finalMetadata = metadataByPath[normalizedPath];
	return {
		path: normalizedPath,
		segments,
		confidenceLabel: formatTopicCategoryConfidence(finalMetadata?.confidence)
	};
}

export function formatTopicCategoryConfidence(confidence: number | null | undefined): string {
	return Number.isFinite(confidence) ? Number(confidence).toFixed(2) : '';
}

function buildTopicCategoryTooltip(
	label: string,
	path: string,
	metadata?: TopicCategoryPathMetadata
): string {
	if (!metadata) return `${label}\n${path}`;
	const lines = [`${label}`, `Path: ${path}`];
	if (metadata.desc?.trim()) lines.push(`Description: ${metadata.desc.trim()}`);
	if (Number.isFinite(metadata.confidence)) {
		lines.push(`Confidence: ${formatTopicCategoryConfidence(metadata.confidence)}`);
	}
	if (metadata.keywords?.length) lines.push(`Keywords: ${metadata.keywords.join(', ')}`);
	if (metadata.create_time?.trim()) lines.push(`Created: ${metadata.create_time.trim()}`);
	return lines.join('\n');
}

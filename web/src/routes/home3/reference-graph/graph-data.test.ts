import { expect, test } from 'bun:test';
import { buildDocumentHierarchy, flattenHierarchy, buildNavigator } from './graph-data';

const document = { id: 416, name: 'Document' };
const labels = { metrics: 'Metrics', references: 'References' };
const metrics = [{ id: 1, name: 'pH', value: '5.5–8.5' }, { id: 2, name: 'Mass', value: '30%' }];
// Separate mentions can refer to the same document, including the source itself.
const references = [{ id: 1, name: 'Document', value: 'ref-1' }, { id: 2, name: 'Document', value: 'ref-2' }];

test('document has exactly two branches, each containing all its records', () => {
	const root = buildDocumentHierarchy(document, metrics, references, labels);
	expect(root.children.map((node) => node.name)).toEqual(['Metrics', 'References']);
	expect(root.children[0].children.map((node) => node.name)).toEqual(['pH', 'Mass']);
	expect(root.children[1].children.map((node) => node.value)).toEqual(['ref-1', 'ref-2']);
	const { nodes, links } = flattenHierarchy(root);
	expect(new Set(nodes.map((node) => node.id)).size).toBe(7);
	expect(links.filter((link) => link.source === root.id)).toHaveLength(2);
	for (const group of root.children) {
		expect(links.filter((link) => link.source === group.id).map((link) => link.target))
			.toEqual(group.children.map((node) => node.id));
	}
});

for (const [metricRecords, referenceRecords] of [[metrics, []], [[], references], [[], []]]) {
	test(`preserves both groups with ${metricRecords.length} metrics and ${referenceRecords.length} references`, () => {
		const root = buildDocumentHierarchy(document, metricRecords, referenceRecords, { metrics: '指标', references: '引用' });
		expect(root.children.map((node) => node.name)).toEqual(['指标', '引用']);
		expect(flattenHierarchy(root).links).toHaveLength(metricRecords.length + referenceRecords.length + 2);
	});
}

test('returning from tree to graph uses the rendered graph positions in the minimap', () => {
	const root = buildDocumentHierarchy(document, metrics, references, labels);
	const positions = Object.fromEntries(flattenHierarchy(root).nodes.map((node, index) =>
		[node.id, [index * 30, (index % 3) * 40] as [number, number]]));
	const graph = buildNavigator(root, 'graph', positions);
	const tree = buildNavigator(root, 'top-bottom', positions);
	expect(buildNavigator(root, 'graph', positions)).toEqual(graph);
	expect(graph).not.toEqual(tree);
	const scale = (graph[1].x - graph[0].x) / 30;
	expect(graph[1].y - graph[0].y).toBeCloseTo(40 * scale);
	expect(graph[3].y).toBeCloseTo(graph[0].y);
	for (const node of graph) {
		expect(node.x).toBeGreaterThanOrEqual(12);
		expect(node.x).toBeLessThanOrEqual(188);
		expect(node.y).toBeGreaterThanOrEqual(12);
		expect(node.y).toBeLessThanOrEqual(118);
	}
});

for (const mode of ['graph', 'top-bottom', 'bottom-top', 'left-right', 'right-left'] as const) {
	test(`${mode} minimap includes both groups and connects leaves to their parent`, () => {
		const root = buildDocumentHierarchy(document, metrics, references, labels);
		const navigator = buildNavigator(root, mode);
		expect(navigator).toHaveLength(7);
		for (const { source, target } of flattenHierarchy(root).links) {
			expect(navigator.find((node) => node.id === target)?.parentId).toBe(source);
		}
		if (mode !== 'graph') {
			const axis = mode === 'top-bottom' || mode === 'bottom-top' ? 'y' : 'x';
			const sign = mode === 'bottom-top' || mode === 'right-left' ? -1 : 1;
			for (const node of navigator.filter((node) => node.parentId)) {
				const parent = navigator.find((candidate) => candidate.id === node.parentId)!;
				expect((node[axis] - parent[axis]) * sign).toBeGreaterThan(0);
			}
		}
	});
}

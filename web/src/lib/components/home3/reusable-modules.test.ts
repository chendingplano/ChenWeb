import { describe, expect, test } from 'bun:test';
import { filterModules, type ReusableModule } from './reusable-modules';

const modules = [
	{ module_id: '20261025-01', module_name: 'Metric Visualizer', description: '显示指标', module_type: 'frontend', prog_language: 'svelte', status: 'active', version: '1.0.0', files: ['ChenWeb/web/metric.svelte'], config_json: { compact: true }, doc_path: 'KnowledgeStore/module-metric.md' },
	{ module_id: '20261024-01', module_name: 'SQL Export', description: 'Database export', module_type: 'script', prog_language: 'python', status: 'retired', version: '2.0.0', files: ['Utils/export.py'], config_json: {}, doc_path: 'KnowledgeStore/module-export.md' }
] as ReusableModule[];

describe('module search', () => {
	test('blank query returns all records', () => {
		expect(filterModules(modules, '  ')).toEqual(modules);
	});
	test('matches names without case sensitivity or surrounding whitespace', () => {
		expect(filterModules(modules, '  VISUALIZER ')).toEqual([modules[0]]);
	});
	test('searches Chinese descriptions, IDs, type, language, status, version, paths and configuration', () => {
		for (const query of ['指标', '20261025-01', 'frontend', 'svelte', 'active', '1.0.0', 'web/metric', 'compact', 'module-metric']) {
			expect(filterModules(modules, query)).toEqual([modules[0]]);
		}
	});
	test('no matches returns an empty list', () => {
		expect(filterModules(modules, 'not registered')).toEqual([]);
	});
});

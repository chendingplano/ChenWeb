import { expect, test } from 'bun:test';
import { foldersFirst } from './navigation-order';

test('folders precede pages while configured order and section order stay stable', () => {
	const nodes = [
		{ id: 'chat', group: 'Workspace' },
		{ id: 'agents', group: 'Workspace', children: [{}] },
		{ id: 'skills', group: 'Workspace', children: [{}] },
		{ id: 'knowledge', group: 'Workspace' },
		{ id: 'admin-page', group: 'Admin' },
		{ id: 'admin-folder', group: 'Admin', children: [{}] }
	];
	expect(foldersFirst(nodes).map((node) => node.id)).toEqual([
		'agents', 'skills', 'chat', 'knowledge', 'admin-folder', 'admin-page'
	]);
	expect(nodes[0].id).toBe('chat');
});

test('nested siblings without section labels put folders first and treat empty children as pages', () => {
	expect(foldersFirst([
		{ id: 'page' }, { id: 'empty', children: [] },
		{ id: 'folder', children: [{}] }, { id: 'last-page' }
	]).map((node) => node.id)).toEqual(['folder', 'page', 'empty', 'last-page']);
});

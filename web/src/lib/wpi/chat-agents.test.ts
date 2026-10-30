import { describe, expect, test } from 'bun:test';
import { CHAT_AGENTS, offeredAgents, pick } from './chat-agents';

describe('chat assistants', () => {
	test('offers the four harness-backed assistants by default', () => {
		expect(CHAT_AGENTS).toEqual(['Claude Code', 'Codex', 'DeepSeek Harness', 'pi']);
	});

	test('offers the configured assistants whose gateway the server has, in configured order', () => {
		expect(offeredAgents(['pi', 'Claude Code'], null)).toEqual(['pi', 'Claude Code']);
		expect(offeredAgents(['pi', 'Codex', 'Claude Code'], ['claude-code', 'pi'])).toEqual(['pi', 'Claude Code']);
		expect(offeredAgents(CHAT_AGENTS, [])).toEqual([]);
	});

	test('keeps the choice, else the default, else the first', () => {
		expect(pick(['a', 'b'], 'b', 'a')).toBe('b');
		expect(pick(['a', 'b'], 'x', 'b')).toBe('b');
		expect(pick(['a', 'b'], 'x', 'z')).toBe('a');
		expect(pick(['a', 'b'], '')).toBe('a');
		expect(pick([], 'a', 'a')).toBe('');
	});
});

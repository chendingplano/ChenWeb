import { describe, expect, test } from 'bun:test';
import { AgentSSEDecoder, applyAgentEvent, emptyAgentLiveState } from './agentServiceStream';

describe('agent service stream', () => {
	test('parses split UTF-8-safe SSE frames and rejects unknown event types', () => {
		const decoder = new AgentSSEDecoder();
		const first = decoder.push('event: answer_delta\r\ndata: {"type":"answer_delta","text":"P');
		expect(first).toEqual([]);
		const second = decoder.push('ump"}\r\n\r\nevent: thinking_delta\ndata: {"type":"thinking_delta","text":"secret"}\n\n');
		expect(second).toEqual([{ type: 'answer_delta', text: 'Pump' }]);
	});

	test('keeps activity separate from answer and only uses verified final sources', () => {
		let state = emptyAgentLiveState();
		state = applyAgentEvent(state, { type: 'activity', status: 'started', tool: 'search_knowledge', toolCallId: 'call-1' });
		state = applyAgentEvent(state, { type: 'answer_delta', text: 'Flow is low.' });
		state = applyAgentEvent(state, { type: 'sources', sources: [{ document_id: '42', knowledge_store_id: '7', source_title: 'Guide', line_start: 10, line_end: 12 }] });
		state = applyAgentEvent(state, { type: 'completion', status: 'completed' });
		expect(state.answer).toBe('Flow is low.');
		expect(state.activity).toHaveLength(1);
		expect(state.sources).toHaveLength(1);
		expect(state.status).toBe('completed');
	});

	test('failure clears transient sources and permission request', () => {
		let state = emptyAgentLiveState();
		state = applyAgentEvent(state, { type: 'permission_request', requestId: 'request-1', tool: 'read_source_passages' });
		state = applyAgentEvent(state, { type: 'sources', sources: [{ document_id: '42', knowledge_store_id: '7' }] });
		state = applyAgentEvent(state, { type: 'completion', status: 'failed' });
		expect(state.sources).toEqual([]);
		expect(state.permission).toBeNull();
	});

	test('handles CRLF boundaries split across network chunks', () => {
		const decoder = new AgentSSEDecoder();
		expect(decoder.push('event: completion\r')).toEqual([]);
		expect(decoder.push('\ndata: {"type":"completion","status":"completed"}\r')).toEqual([]);
		expect(decoder.push('\n\r\n')).toEqual([{ type: 'completion', status: 'completed' }]);
	});
});

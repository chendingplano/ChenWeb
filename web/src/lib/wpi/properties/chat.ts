// The chat-composer and chat-panel property editors (KnowledgeStore
// code_modules/interpreter-chat-composer.md and interpreter-chat-panel.md,
// Property editor). An absent token list means all of them, in order.
import { m } from '$lib/paraglide/messages.js';
import { CHAT_AGENTS } from '../chat-agents';
import type { Json } from '../types';
import { allUnits, elementPlacement, elementSize } from './shared';
import type { EditorDef } from './types';

export const chatControls = ['agent', 'model', 'prompt', 'text_editor', 'new_session'];
export const chatTabs = ['chat', 'dialog'];

function controlLabel(token: string): string {
	switch (token) {
		case 'agent': return m.wpi_chat_control_agent();
		case 'model': return m.wpi_chat_control_model();
		case 'prompt': return m.wpi_chat_control_prompt();
		case 'text_editor': return m.wpi_chat_control_text_editor();
		case 'new_session': return m.wpi_chat_control_new_session();
		default: return token;
	}
}

function tabLabel(token: string): string {
	return token === 'chat' ? m.wpi_chat_tab_chat() : token === 'dialog' ? m.wpi_chat_tab_dialog() : token;
}

function chosen(value: Json | undefined, all: string[]): string[] {
	return Array.isArray(value) ? value.filter((v): v is string => typeof v === 'string') : all;
}

export const chatComposer: EditorDef = {
	groups: [
		elementPlacement,
		{
			id: 'message_box',
			label: m.wpi_chat_group_message_box,
			properties: [
				{ name: 'placeholder', label: m.wpi_input_attr_placeholder, editor: 'localized-text' },
				{ name: 'rows', label: m.wpi_chat_attr_rows, editor: 'number', options: { min: 1, max: 20, integer: true } },
				{ name: 'slash_commands', label: m.wpi_chat_attr_slash_commands, editor: 'switch', read: (v) => v !== false }
			]
		},
		{
			id: 'toolbar',
			label: m.wpi_chat_group_toolbar,
			properties: [{ name: 'toolbar', label: m.wpi_chat_attr_toolbar, editor: 'ordered-multi-picker', options: { choices: chatControls, choiceLabel: controlLabel, minItems: 0 } }]
		},
		{
			id: 'assistant',
			label: m.wpi_chat_group_assistant,
			properties: [
				{ name: 'agents', label: m.wpi_chat_attr_agents, editor: 'ordered-multi-picker', options: { choices: CHAT_AGENTS, minItems: 1 } },
				{ name: 'default_agent', label: m.wpi_chat_attr_default_agent, editor: 'value-picker', options: (ctx) => ({ choices: chosen(ctx.get('agents'), CHAT_AGENTS), emptyLabel: m.wpi_chat_choice_first }) },
				{ name: 'default_model', label: m.wpi_chat_attr_default_model, editor: 'text' }
			]
		},
		{
			...elementSize(),
			properties: [
				...elementSize().properties,
				{ name: 'message_box_height', label: m.wpi_chat_message_box_height, editor: 'length', options: { units: allUnits } },
				{ name: 'toolbar_height', label: m.wpi_chat_toolbar_height, editor: 'length', options: { units: allUnits } }
			]
		}
	]
};

export const chatPanel: EditorDef = {
	groups: [
		elementPlacement,
		{
			id: 'tabs',
			label: m.wpi_chat_group_tabs,
			properties: [
				{ name: 'tabs', label: m.wpi_chat_attr_tabs, editor: 'ordered-multi-picker', options: { choices: chatTabs, choiceLabel: tabLabel, minItems: 1 } },
				{ name: 'initial_tab', label: m.wpi_chat_attr_initial_tab, editor: 'value-picker', options: (ctx) => ({ choices: chosen(ctx.get('tabs'), chatTabs), choiceLabel: tabLabel, emptyLabel: m.wpi_chat_choice_first }) }
			]
		},
		elementSize()
	]
};

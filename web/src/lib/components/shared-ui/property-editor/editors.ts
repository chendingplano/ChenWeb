// The value editor component for each value editor ID.
import type { Component } from 'svelte';
import type { ValueEditorId } from '$lib/wpi/properties/types';
import type { ValueEditorProps } from '../value-editors/types';
import ColorPicker from '../value-editors/ColorPicker.svelte';
import ColumnListEditor from '../value-editors/ColumnListEditor.svelte';
import DataSourceEditor from '../value-editors/DataSourceEditor.svelte';
import JsonEditor from '../value-editors/JsonEditor.svelte';
import LengthEditor from '../value-editors/LengthEditor.svelte';
import LocalizedTextEditor from '../value-editors/LocalizedTextEditor.svelte';
import MultilineTextEditor from '../value-editors/MultilineTextEditor.svelte';
import NumberEditor from '../value-editors/NumberEditor.svelte';
import OrderedMultiPicker from '../value-editors/OrderedMultiPicker.svelte';
import ReadOnlyValue from '../value-editors/ReadOnlyValue.svelte';
import StringListEditor from '../value-editors/StringListEditor.svelte';
import SwitchEditor from '../value-editors/SwitchEditor.svelte';
import TextEditor from '../value-editors/TextEditor.svelte';
import ValidationEditor from '../value-editors/ValidationEditor.svelte';
import ValuePicker from '../value-editors/ValuePicker.svelte';

export const valueEditors: Record<ValueEditorId, Component<ValueEditorProps>> = {
	text: TextEditor,
	'multiline-text': MultilineTextEditor,
	'localized-text': LocalizedTextEditor,
	number: NumberEditor,
	switch: SwitchEditor,
	'value-picker': ValuePicker,
	'ordered-multi-picker': OrderedMultiPicker,
	'string-list': StringListEditor,
	color: ColorPicker,
	length: LengthEditor,
	'data-source': DataSourceEditor,
	'column-list': ColumnListEditor,
	validation: ValidationEditor,
	'read-only': ReadOnlyValue,
	json: JsonEditor
};

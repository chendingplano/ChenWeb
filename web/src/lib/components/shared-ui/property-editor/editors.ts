// The value editor component for each value editor ID.
import type { Component } from 'svelte';
import type { ValueEditorId } from '$lib/wpi/properties/types';
import type { ValueEditorProps } from '../value-editors/types';
import ColorPicker from '../value-editors/ColorPicker.svelte';
import ColumnListEditor from '../value-editors/ColumnListEditor.svelte';
import DataSourceEditor from '../value-editors/DataSourceEditor.svelte';
import FieldPickerEditor from '../value-editors/FieldPickerEditor.svelte';
import EmojiPickerEditor from '../value-editors/EmojiPickerEditor.svelte';
import IconPickerEditor from '../value-editors/IconPickerEditor.svelte';
import JsonEditor from '../value-editors/JsonEditor.svelte';
import LengthEditor from '../value-editors/LengthEditor.svelte';
import LocalizedTextEditor from '../value-editors/LocalizedTextEditor.svelte';
import MultilineTextEditor from '../value-editors/MultilineTextEditor.svelte';
import NumberEditor from '../value-editors/NumberEditor.svelte';
import OrderedMultiPicker from '../value-editors/OrderedMultiPicker.svelte';
import ReadOnlyValue from '../value-editors/ReadOnlyValue.svelte';
import SortOrderEditor from '../value-editors/SortOrderEditor.svelte';
import StringChoicesEditor from '../value-editors/StringChoicesEditor.svelte';
import StringListEditor from '../value-editors/StringListEditor.svelte';
import SwitchEditor from '../value-editors/SwitchEditor.svelte';
import TablePickerEditor from '../value-editors/TablePickerEditor.svelte';
import TextEditor from '../value-editors/TextEditor.svelte';
import StringValueEditor from '../value-editors/StringValueEditor.svelte';
import ValidationEditor from '../value-editors/ValidationEditor.svelte';
import ValidatorListEditor from '../value-editors/ValidatorListEditor.svelte';
import ValuePicker from '../value-editors/ValuePicker.svelte';

export const valueEditors: Record<ValueEditorId, Component<ValueEditorProps>> = {
	text: TextEditor,
	'string-editor': StringValueEditor,
	'multiline-text': MultilineTextEditor,
	'localized-text': LocalizedTextEditor,
	number: NumberEditor,
	switch: SwitchEditor,
	'value-picker': ValuePicker,
	'ordered-multi-picker': OrderedMultiPicker,
	'string-list': StringListEditor,
	'string-choices': StringChoicesEditor,
	color: ColorPicker,
	length: LengthEditor,
	'icon-picker': IconPickerEditor,
	'emoji-picker': EmojiPickerEditor,
	'table-picker': TablePickerEditor,
	'field-picker': FieldPickerEditor,
	'data-source': DataSourceEditor,
	'column-list': ColumnListEditor,
	'sort-order-editor': SortOrderEditor,
	validation: ValidationEditor,
	'validator-list': ValidatorListEditor,
	'read-only': ReadOnlyValue,
	json: JsonEditor
};

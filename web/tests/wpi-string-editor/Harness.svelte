<script lang="ts">
 import StringValueEditor from '../../src/lib/components/shared-ui/value-editors/StringValueEditor.svelte';
 import type { Json } from '../../src/lib/wpi/types';
 import type { JsonObject } from '../../src/lib/wpi/types';
 import type { ElementClassSummary } from '../../src/lib/wpi/api';
 import type { ValidatorClass } from '../../src/lib/wpi/validators';
 import PropertyEditor from '../../src/lib/components/shared-ui/property-editor/PropertyEditor.svelte';
 import { elementEditor } from '../../src/lib/wpi/properties';
 import { elementSource } from '../../src/lib/wpi/properties/sources';
 import { withValidatorGroups, withValidatorSource } from '../../src/lib/wpi/properties/validators';
 import { elementClassDefinitions, withClassEditors } from '../../src/lib/wpi/properties/class-definition';
 let value = $state<Json | undefined>('Hello');
 let emptyAsNull = $state(true);
 let attrs = $state<JsonObject>({});
 const cls = { attributes: { editor: { scope: 'class', default_value: 'string-editor' }, label: { scope: 'instance', value_editor: { id: 'string-editor', options: { mode: 'multilingual', languages: ['en', 'zh-cn'] } } }, editor_options: { default_value: { mode: 'single', choices: [], languages: ['en', 'zh-cn'], emptyAsNull: true, commitOnBlur: false } } } } as unknown as ElementClassSummary;
 const validator = { descriptor_id: 'string', code_module_id: 'string', category: 'primitive', constraints: {}, description: { en: 'String' } } as unknown as ValidatorClass;
 const source = $derived(withValidatorSource(elementSource(attrs, cls, {}, next => attrs = next)));
</script>
<label><input type="checkbox" bind:checked={emptyAsNull} />empty as null</label>
<div id="standalone"><StringValueEditor id="field" label="Name" {value} options={{ emptyAsNull, choices: ['red', 'blue'] }} onChange={(next) => value = next} /></div>
<pre id="result">{JSON.stringify(value)}</pre>
<PropertyEditor title="Author" def={withValidatorGroups(withClassEditors(elementEditor('value-editor'), elementClassDefinitions(cls)), source.get('validators'))} {source} context={{triggerActions: [], pageLayouters: [], validatorClasses: [validator]}} idPrefix="test" />
<pre id="attrs">{JSON.stringify(attrs)}</pre>

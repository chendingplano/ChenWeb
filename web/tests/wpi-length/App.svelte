<script lang="ts">
 import LengthEditor from '../../src/lib/components/shared-ui/value-editors/LengthEditor.svelte';
 import PropertyEditor from '../../src/lib/components/shared-ui/property-editor/PropertyEditor.svelte';
 import { valueEditor } from '../../src/lib/wpi/properties/elements';
 import { elementSource } from '../../src/lib/wpi/properties/sources';
 import type { JsonObject } from '../../src/lib/wpi/types';
 import type { ElementClassSummary } from '../../src/lib/wpi/api';
 import type { Json } from '../../src/lib/wpi/types';
 let value = $state<Json | undefined>('12px');
 let pageValue = $state<Json | undefined>(undefined);
 let attrs = $state<JsonObject>({ width: '24px', label: 'Test' });
 const cls: ElementClassSummary = { descriptor_id: 'length-editor', version: 1, status: 'active', description: null, attributes: { width: { default_value: 'elastic' }, label: {default_value: 'Label'}, editor: {scope: 'class', default_value: 'length'}, editor_options: {default_value: {keywords:['auto'], units:['px','%']}} } };
 const source = $derived(elementSource(attrs, cls, {}, (next) => attrs = next));
</script>
<LengthEditor id="length" label="Width" {value} options={{units:['px','%'],keywords:['auto']}} onChange={(v) => value = v} />
<output id="length-output">{value ?? 'unset'}</output><button>Outside</button>

<PropertyEditor title="Properties" def={valueEditor} {source} context={{triggerActions: [], pageLayouters: []}} idPrefix="properties" />
<output id="attributes">{JSON.stringify(attrs)}</output>

<div id="page-length">
 <LengthEditor id="page-length-value" label="Page width" value={pageValue} options={{ units:['px','%'], keywords:['auto'], defaultValue:'300px', useDefault: pageValue == null, displayType: source.get('editor_options.displayType') === true }} onChange={(v) => pageValue = v} />
 <output id="page-value">{pageValue ?? 'unset'}</output>
</div>

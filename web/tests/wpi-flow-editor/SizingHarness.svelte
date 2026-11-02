<script lang="ts">
 import WpiProperties from '../../src/lib/components/shared-ui/canvas/WpiProperties.svelte';
 import PagePreview from '../../src/lib/components/shared-ui/canvas/PagePreview.svelte';
 import type { FlowNode } from '../../src/lib/types/flow';
 import type { ElementClassSummary } from '../../src/lib/wpi/api';
 const attrs = { width: { default_value: 'auto' }, height: { default_value: 'auto' }, min_width: {}, max_width: {} };
 let selected = $state('button');
 let nodes = $state<FlowNode[]>(['button', 'input'].map((id, i) => ({ id, type:'wpi-element', position:{x: i * 10,y:0}, data:{element:id,pane:'main',attrs:id === 'input' ? {width:'200px',max_width:'200px'} : {width:'150px'},class:{descriptor_id:id,code_module_id:id}} })));
 const page = { descriptor_id:'sizing', page_layouter:{layouter:'single-pane',root:{pane_id:'main'}}, panels:{main:{elem_layouter:{layouter:'flow-layouter',config:{}}}} };
 let node = $derived(nodes.find(n=>n.id===selected)!);
 let cls = $derived({descriptor_id:selected,version:1,status:'active',description:null,code_module_id:selected,attributes:attrs} as ElementClassSummary);
 const noop = () => {};
</script>
<button id="select-button" onclick={() => selected='button'}>Button</button>
<button id="select-input" onclick={() => selected='input'}>Input</button>
<div style="width:400px; height:500px">
<WpiProperties page={page} mode={null} selectedPane={null} node={node as any} elementClass={cls} triggerActions={[]} pageLayouters={[]} positions={{}}
 onPage={noop} onNode={(id,data)=>nodes=nodes.map(n=>n.id===id?{...n,data}:n)} onLayouter={noop} onRename={noop} onSelectPane={noop}
 onSplit={noop} onAdd={noop} onMerge={noop} onDelete={noop} onEditStart={noop} onEditEnd={noop} />
</div>
<div style="position:relative;width:800px;height:400px">
<PagePreview descriptor={page} {nodes} selectedId={null} selectedPane={null} size={{width:800,height:400}} zoom={1} darkMode={false}
 onResizeStart={noop} onResize={noop} onSelect={noop} onSelectPane={noop} onMeasure={noop} onPaneResize={noop} onPageResize={noop} onPageCancel={noop} onMeasurePanes={noop} />
</div>

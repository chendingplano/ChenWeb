<script lang="ts">
 import PropertiesPanel from '../../src/lib/components/shared-ui/canvas/PropertiesPanel.svelte';
 import PagePreview from '../../src/lib/components/shared-ui/canvas/PagePreview.svelte';
 import type { FlowNode, NodeType } from '../../src/lib/types/flow';
 import type { ElementClassSummary } from '../../src/lib/wpi/api';
 const attrs = { width: { default_value: 'auto' }, height: { default_value: 'auto' }, min_width: {}, max_width: {} };
 let selected = $state('button');
 let nodes = $state<FlowNode[]>(['button', 'input'].map((id, i) => ({ id, type:'wpi-element', position:{x: i * 10,y:0}, data:{element:id,pane:'main',attrs:id === 'input' ? {width:'200px',max_width:'200px'} : {width:'150px'},class:{descriptor_id:id,code_module_id:id}} })));
 const page = { descriptor_id:'sizing', page_layouter:{layouter:'single-pane',root:{pane_id:'main'}}, panels:{main:{elem_layouter:{layouter:'flow-layouter',config:{}}}} };
 const kind: NodeType = { id:'wpi-element',label:'element',category:'',icon:'',inputs:[],outputs:[],defaultData:{} };
 let node = $derived(nodes.find(n=>n.id===selected)!);
 let cls = $derived({descriptor_id:selected,version:1,status:'active',description:null,code_module_id:selected,attributes:attrs} as ElementClassSummary);
 const noop = () => {};
</script>
<button id="select-button" onclick={() => selected='button'}>Button</button>
<button id="select-input" onclick={() => selected='input'}>Input</button>
<PropertiesPanel {node} nodeType={kind} elementClass={cls} measuredSize={{width:150,height:36}} onUpdate={(id,data)=>nodes=nodes.map(n=>n.id===id?{...n,data}:n)} />
<div style="position:relative;width:800px;height:400px">
<PagePreview descriptor={page} {nodes} selectedId={null} selectedPane={null} size={{width:800,height:400}} zoom={1} darkMode={false}
 onResizeStart={noop} onResize={noop} onSelect={noop} onSelectPane={noop} onMeasure={noop} onPaneResize={noop} onPageResize={noop} onPageCancel={noop} onMeasurePanes={noop} />
</div>

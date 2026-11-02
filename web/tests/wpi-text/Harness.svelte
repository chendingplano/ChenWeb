<script lang="ts">
	import { AppRuntime, setRuntime } from '../../src/lib/wpi/runtime.svelte';
	import PanelView from '../../src/lib/wpi/PanelView.svelte';
	import PropertyEditor from '../../src/lib/components/shared-ui/property-editor/PropertyEditor.svelte';
	import { elementEditor, moduleFallbacks } from '../../src/lib/wpi/properties';
	import { elementSource } from '../../src/lib/wpi/properties/sources';
	import { overwriteGetLocale } from '../../src/lib/paraglide/runtime';
	import bundleData from './bundle.json';
	const cls = bundleData.pages['text-r2-example'].panels.main.elem_list[0].class;
	import type { Bundle, JsonObject } from '../../src/lib/wpi/types';
	import type { ElementAttribute, ElementClassSummary } from '../../src/lib/wpi/api';
	let locale = $state<'en' | 'zh-cn'>('en');
	overwriteGetLocale(() => locale);
	const bundle = $state(bundleData as unknown as Bundle);
	const rt = new AppRuntime(bundle, true);
	setRuntime(rt);
	rt.start();
	let hidden = $state(false);
	let mounted = $state(true);
	let narrow = $state(false);
	let dark = $state(false);
	let attrs = $state<JsonObject>({
		text: { en: 'Editor text', 'zh-cn': '编辑文本' },
		label: { en: 'Caption', 'zh-cn': '标题' },
		icon: 'info',
		future_option: 'preserved'
	});
	const {
		descriptor_id,
		schema_version,
		code_module_id,
		description,
		implement_notes,
		...attributes
	} = cls;
	const definitions = {
		...attributes,
		help_tip: {
			...attributes.help_tip,
			default_value: { en: 'Default tooltip', 'zh-cn': '默认提示' }
		},
		font_style: {
			help_tip: { en: 'Fixed style', 'zh-cn': '固定样式' },
			default_value: 'normal',
			instance_modifiable: false
		},
		future_option: { help_tip: { en: 'Future option', 'zh-cn': '未来选项' } },
		max_height: { default_value: '150px' }
	} as Record<string, ElementAttribute>;
	const summary = { descriptor_id, version: 1, status: 'active', description, code_module_id, attributes: definitions } as unknown as ElementClassSummary;
	const flowConfig = { max_elem_width: '300px', max_row_height: '200px' };
	function edit(v: JsonObject) {
		attrs = v;
		bundle.pages['text-r2-example'].panels!.main.elem_list![0] = {
			element: 'text',
			class: { ...cls, ...definitions },
			...v,
			width: '100%',
			max_width: '640px'
		};
	}
</script>

<svelte:window
	onkeydown={(e) => {
		if (e.key === 'F2') locale = locale === 'en' ? 'zh-cn' : 'en';
	}}
/>
<button id="locale" onclick={() => (locale = locale === 'en' ? 'zh-cn' : 'en')}>Locale</button>
<button id="unmount" onclick={() => (mounted = !mounted)}>Unmount</button>
<button id="hide" onclick={() => (hidden = !hidden)}>Hide</button>
<button id="narrow" onclick={() => (narrow = !narrow)}>Narrow</button>
<button id="theme" onclick={() => (dark = !dark)}>Theme</button>
<button
	id="update"
	onclick={() => {
		rt.write(rt.root!, '$input_doc.zero', 42);
	}}>Update</button
>
<div
	class:dark
	class="shell"
	style:--background={dark ? '#161616' : '#fff'}
	style:--foreground={dark ? '#f5f5f5' : '#171717'}
	style:color={dark ? '#f5f5f5' : '#171717'}
	style:background={dark ? '#161616' : '#fff'}
>
	<div id="host" style:width={narrow ? '220px' : '420px'} style:display={hidden ? 'none' : 'block'}>
		{#if mounted}<PanelView instanceId={rt.root!} paneId="main" />{/if}
	</div>
</div>
<div id="editor" style="width:340px">
	<PropertyEditor
		title="text"
		def={elementEditor('text')}
		source={elementSource(attrs, summary, moduleFallbacks('text'), edit)}
		context={{ triggerActions: [], pageLayouters: [], pane: 'main', flowConfig }}
		idPrefix="text-prop"
	/>
</div>
<pre id="attrs">{JSON.stringify(attrs)}</pre>

<style>
	:global(body) {
		font-family: Arial, sans-serif;
		margin: 20px;
		--cv-text: #171717;
		--cv-text-muted: #444;
		--cv-accent: #4338ca;
		--cv-border: #aaa;
		--cv-surface: #fff;
		--border: #aaa;
		--ring: #4f46e5;
	}
	.shell {
		user-select: none;
		padding: 12px;
	}
</style>

<script lang="ts">
	import { onDestroy, onMount } from 'svelte';

	export let syntax: string;
	export let data: string;
	export let width: string | number = '100%';
	export let height: string | number = '100%';
	export let renderToken: number = 0;
	export let theme: string = '';
	export let palette: string | string[] = '';

	let containerEl: HTMLDivElement;
	type InfographicInstance = {
		render: (options: string | Record<string, unknown>) => void;
		destroy: () => void;
		toDataURL: (options?: { type: 'png' } | { type: 'svg'; embedResources?: boolean }) => Promise<string>;
	};
	let infographic: InfographicInstance | null = null;
	let parseSyntaxFn: ((input: string) => { options?: Record<string, unknown> }) | null = null;
	let lastRendered = '';
	let lastRenderToken = -1;

	const downloadDataURL = (dataURL: string, filename: string) => {
		const link = document.createElement('a');
		link.href = dataURL;
		link.download = filename;
		document.body.appendChild(link);
		link.click();
		document.body.removeChild(link);
	};

	export const downloadPNG = async (filename = 'infographic.png') => {
		if (!infographic) return;
		const dataURL = await infographic.toDataURL({ type: 'png' });
		downloadDataURL(dataURL, filename);
	};

	export const downloadSVG = async (filename = 'infographic.svg') => {
		if (!infographic) return;
		const dataURL = await infographic.toDataURL({ type: 'svg', embedResources: true });
		downloadDataURL(dataURL, filename);
	};

	const renderContent = () => {
		if (!infographic) return;
		const syntaxBlock = syntax.trim();
		const dataBlock = data.trim();
		const baseSyntax = dataBlock.startsWith('data') ? `${syntaxBlock}\n${dataBlock}` : `${syntaxBlock}\ndata\n${dataBlock}`;
		let finalSyntax = baseSyntax;
		const themeType = theme.trim();
		const themePalette = Array.isArray(palette) ? palette : palette.trim();
		if (themeType || themePalette) {
			finalSyntax += '\n' + 'theme';
			if (themeType) {
				finalSyntax += `\n  type ${themeType}`;
			}
			if (themePalette) {
				finalSyntax += `\n  palette ${Array.isArray(themePalette) ? themePalette.join(' ') : themePalette}`;
			}
		}
		if (finalSyntax === lastRendered && renderToken === lastRenderToken) return;
		if (parseSyntaxFn) {
			const parsed = parseSyntaxFn(baseSyntax)?.options ?? {};
			const options = { ...parsed };
			if (themeType) {
				options.theme = themeType;
			} else {
				delete options.theme;
			}
			if (themePalette) {
				options.themeConfig = { ...(options.themeConfig as Record<string, unknown> | undefined), palette: themePalette };
			}
			infographic.render(options);
		} else {
			infographic.render(finalSyntax);
		}
		lastRendered = finalSyntax;
		lastRenderToken = renderToken;
	};

	onMount(async () => {
		const { Infographic, parseSyntax } = await import('@antv/infographic');
		parseSyntaxFn = parseSyntax as (input: string) => { options?: Record<string, unknown> };
		infographic = new Infographic({
			container: containerEl,
			width,
			height
		});

		renderContent();
	});

	$: if (infographic && syntax && data && renderToken >= 0) {
		renderContent();
	}

	onDestroy(() => {
		infographic?.destroy();
		infographic = null;
		parseSyntaxFn = null;
		lastRendered = '';
		lastRenderToken = -1;
	});
</script>

<div bind:this={containerEl} class="infographic-container"></div>

<style>
	.infographic-container {
		width: 100%;
		height: 100%;
		min-height: 360px;
	}
</style>

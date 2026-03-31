<script lang="ts">
	import { onMount } from 'svelte';
	import InfographicWrapper from './InfographicWrapper.svelte';

	type PalettePreset = {
		id: string;
		label: string;
		value: string | string[];
	};

	let templates = ['list-row-simple-horizontal-arrow'];
	let themes = ['light'];
	let palettePresets: PalettePreset[] = [
		{ id: '', label: 'default', value: '' },
		{ id: 'spectral', label: 'spectral', value: 'spectral' },
		{
			id: 'sunset',
			label: 'Sunset',
			value: ['#7c2d12', '#ea580c', '#fb923c', '#fdba74', '#ffedd5']
		},
		{
			id: 'deep-sea',
			label: 'Deep Sea',
			value: ['#0f172a', '#1e3a8a', '#0369a1', '#0ea5e9', '#7dd3fc']
		},
		{
			id: 'slate-mint',
			label: 'Slate Mint',
			value: ['#0f172a', '#334155', '#38bdf8', '#a7f3d0', '#fbbf24']
		}
	];
	let selectedTemplate = 'list-row-simple-horizontal-arrow';
	let selectedTheme = 'light';
	let selectedPaletteId = '';
	let draftData = `data
  lists
    - label Step 1
      desc Start
    - label Step 2
      desc In Progress
    - label Step 3
      desc Complete`;

	let renderedSyntax = `infographic ${selectedTemplate}`;
	let renderedData = draftData;
	let renderedTheme = selectedTheme;
	let renderedPaletteId = selectedPaletteId;
	let renderedPalette: string | string[] = '';
	let renderToken = 1;
	let infographicRef:
		| {
				downloadPNG: (filename?: string) => Promise<void>;
				downloadSVG: (filename?: string) => Promise<void>;
		  }
		| undefined;

	onMount(async () => {
		const { getTemplates, getThemes } = await import('@antv/infographic');
		const allTemplates = [...getTemplates()].sort();
		templates = allTemplates;
		if (!allTemplates.includes(selectedTemplate) && allTemplates.length > 0) {
			selectedTemplate = allTemplates[0];
		}

		const allThemes = [...getThemes()].sort();
		themes = ['light', ...allThemes];
	});

	const handleRender = () => {
		renderedSyntax = `infographic ${selectedTemplate}`;
		renderedData = draftData;
		renderedTheme = selectedTheme;
		renderedPaletteId = selectedPaletteId;
		const selectedPalettePreset = palettePresets.find((p) => p.id === selectedPaletteId);
		renderedPalette = selectedPalettePreset?.value ?? '';
		renderToken += 1;
	};

	const handleDownloadPNG = async () => {
		await infographicRef?.downloadPNG(`infographic-${selectedTemplate}.png`);
	};

	const handleDownloadSVG = async () => {
		await infographicRef?.downloadSVG(`infographic-${selectedTemplate}.svg`);
	};
</script>

<main class="page">
	<h1>Infographic Editor 01</h1>

	<section class="panel editor-panel">
		<h2>Editor Panel</h2>
		<div class="field">
			<label for="template">Template (syntax)</label>
			<select id="template" bind:value={selectedTemplate}>
				{#each templates as template}
					<option value={template}>{template}</option>
				{/each}
			</select>
		</div>

		<div class="field field-row">
			<div class="field">
				<label for="theme">Theme</label>
				<select id="theme" bind:value={selectedTheme}>
					{#each themes as theme}
						<option value={theme}>{theme}</option>
					{/each}
				</select>
			</div>

			<div class="field">
				<label for="palette">Palette</label>
				<select id="palette" bind:value={selectedPaletteId}>
					{#each palettePresets as preset}
						<option value={preset.id}>{preset.label}</option>
					{/each}
				</select>
			</div>
		</div>

		<div class="field">
			<label for="data">Data</label>
			<textarea
				id="data"
				bind:value={draftData}
				spellcheck="false"
				placeholder="Enter infographic data block..."
			></textarea>
		</div>

		<button class="render-btn" onclick={handleRender}>Render</button>
	</section>

	<section class="panel canvas-panel">
		<h2>Canvas Panel</h2>
		<div class="canvas-actions">
			<button class="secondary-btn" onclick={handleDownloadPNG}>Download PNG</button>
			<button class="secondary-btn" onclick={handleDownloadSVG}>Download SVG</button>
		</div>
		<div class="canvas">
			<InfographicWrapper
				bind:this={infographicRef}
				syntax={renderedSyntax}
				data={renderedData}
				theme={renderedTheme === 'light' ? '' : renderedTheme}
				palette={renderedPalette}
				{renderToken}
			/>
		</div>
		<div class="hint">
			Rendering syntax: <code>{renderedSyntax}</code>
		</div>
		<div class="hint">
			Theme: <code>{renderedTheme}</code> | Palette: <code>{renderedPaletteId || 'default'}</code>
		</div>
	</section>
</main>

<style>
	.page {
		padding: 1.5rem;
		display: grid;
		gap: 1.25rem;
	}

	h1 {
		margin: 0;
		font-size: 1.5rem;
	}

	h2 {
		margin: 0;
		font-size: 1.05rem;
	}

	.panel {
		display: grid;
		gap: 0.9rem;
		border: 1px solid #e5e7eb;
		border-radius: 12px;
		padding: 1rem;
		background: #fff;
	}

	.field {
		display: grid;
		gap: 0.4rem;
	}

	.field-row {
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.75rem;
	}

	label {
		font-weight: 600;
		font-size: 0.9rem;
		color: #374151;
	}

	select,
	textarea {
		border: 1px solid #d1d5db;
		border-radius: 8px;
		padding: 0.65rem 0.75rem;
		font: inherit;
		background: #fff;
	}

	textarea {
		min-height: 200px;
		font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
		font-size: 0.85rem;
		line-height: 1.45;
		resize: vertical;
	}

	.render-btn {
		justify-self: start;
		border: 1px solid #1d4ed8;
		background: #2563eb;
		color: #fff;
		padding: 0.55rem 1rem;
		border-radius: 8px;
		font-weight: 600;
		cursor: pointer;
	}

	.render-btn:hover {
		background: #1d4ed8;
	}

	.canvas {
		height: 420px;
		border: 1px solid #e5e7eb;
		border-radius: 12px;
		padding: 1rem;
		background: #fafafa;
	}

	.hint {
		font-size: 0.85rem;
		color: #4b5563;
		word-break: break-all;
	}

	.canvas-actions {
		display: flex;
		gap: 0.6rem;
		flex-wrap: wrap;
	}

	.secondary-btn {
		border: 1px solid #cbd5e1;
		background: #fff;
		color: #111827;
		padding: 0.45rem 0.85rem;
		border-radius: 8px;
		font-weight: 600;
		cursor: pointer;
	}

	.secondary-btn:hover {
		background: #f8fafc;
	}

	code {
		font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
	}

	@media (max-width: 720px) {
		.field-row {
			grid-template-columns: 1fr;
		}
	}
</style>

// Colors of the canvas editors (WPI editor, Flow) in dark and light mode. Each editor sets them
// on its root as CSS variables (--cv-panel, --cv-text, …) with canvasThemeVars(); the palette,
// tool bar, properties panel, nodes and dialogs inside read them with var(--cv-…).
// Dialogs rendered outside the editor's root set them again on their own root.

const DARK = {
	canvas: '#0d1117', // canvas background
	panel: '#111827', // side panels and dialogs
	raised: '#161b27', // tool bar, nodes, minimap, cards
	surface: '#1e2535', // inputs, list items, chips
	'surface-hover': '#273046',
	field: '#0d1117', // inline edit fields
	divider: '#1e2a3a', // lines between areas
	border: '#374151', // borders of inputs and nodes
	text: '#e2e8f0',
	'text-2': '#94a3b8',
	'text-muted': '#6b7280',
	'text-faint': '#4b5563',
	accent: '#6366f1',
	'accent-2': '#818cf8',
	warn: '#fbbf24',
	'error-bg': '#1f1313',
	grid: '#374151', // background dots
	hover: 'rgba(255,255,255,0.05)',
	shadow: 'rgba(0,0,0,0.4)',
	backdrop: 'rgba(0,0,0,0.7)'
};

const LIGHT: typeof DARK = {
	canvas: '#f8fafc',
	panel: '#ffffff',
	raised: '#ffffff',
	surface: '#f1f5f9',
	'surface-hover': '#e2e8f0',
	field: '#ffffff',
	divider: '#e5e7eb',
	border: '#d1d5db',
	// All text is black in light mode; size and weight, not grey, set the levels apart.
	text: '#000000',
	'text-2': '#000000',
	'text-muted': '#000000',
	'text-faint': '#000000',
	accent: '#6366f1',
	'accent-2': '#4f46e5',
	warn: '#d97706',
	'error-bg': '#fef2f2',
	grid: '#cbd5e1',
	hover: 'rgba(15,23,42,0.05)',
	shadow: 'rgba(15,23,42,0.12)',
	backdrop: 'rgba(15,23,42,0.4)'
};

// The colors as CSS variable declarations, for a style attribute.
export function canvasThemeVars(dark: boolean): string {
	return Object.entries(dark ? DARK : LIGHT)
		.map(([name, value]) => `--cv-${name}:${value};`)
		.join(' ');
}

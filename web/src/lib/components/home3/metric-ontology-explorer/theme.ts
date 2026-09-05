// Token set for the Metric Ontology Explorer panes, derived from the
// knowledge view's `darkMode`. Values mirror the palette
// `home3/knowledge/+page.svelte` derives for its own shell so the explorer
// sits flush with the rest of the page. No new global token is introduced.

export type ExplorerTokens = {
	pageBg: string;
	panelBg: string;
	cardBg: string;
	border: string;
	borderStrong: string;
	accent: string;
	accentTint: string;
	textPrimary: string;
	textSecondary: string;
	textMuted: string;
	hoverBg: string;
	ok: string;
	warn: string;
	err: string;
};

export function explorerTokens(dark: boolean): ExplorerTokens {
	return dark
		? {
				pageBg: '#171B26',
				panelBg: '#1C212C',
				cardBg: '#232936',
				border: '#2D3348',
				borderStrong: '#3A4160',
				accent: '#818CF8',
				accentTint: 'rgba(129,140,248,0.15)',
				textPrimary: '#E2E8F0',
				textSecondary: '#94A3B8',
				textMuted: '#64748B',
				hoverBg: 'rgba(129,140,248,0.08)',
				ok: '#4ADE80',
				warn: '#FBBF24',
				err: '#F87171'
			}
		: {
				pageBg: '#F2F4F7',
				panelBg: '#FFFFFF',
				cardBg: '#F8FAFC',
				border: '#E4E6EB',
				borderStrong: '#CBD2DC',
				accent: '#6366F1',
				accentTint: 'rgba(99,102,241,0.10)',
				textPrimary: '#111827',
				textSecondary: '#6B7280',
				textMuted: '#9CA3AF',
				hoverBg: 'rgba(99,102,241,0.06)',
				ok: '#16A34A',
				warn: '#B45309',
				err: '#DC2626'
			};
}

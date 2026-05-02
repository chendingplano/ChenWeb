import type {
	SummaryCategoryNode,
	SummaryRecordCard,
	SummaryTreeRecord
} from './summary-types';

export const MOCK_SUMMARY_GRAPH_NODES: SummaryCategoryNode[] = [
	{
		id: 'finance',
		categoryPath: 'finance',
		label: 'Finance',
		metadata: {
			desc: 'High-level financial reporting and tax material.',
			category_type: 'domain',
			confidence: 0.92,
			keywords: ['finance', 'tax', 'audit'],
			create_time: '20260501-090000'
		},
		childIds: ['finance/tax', 'finance/payroll'],
		summaryIds: [],
		expanded: true
	},
	{
		id: 'finance/tax',
		categoryPath: 'finance/tax',
		label: 'Tax',
		metadata: {
			desc: 'Tax summaries grouped by filing and compliance themes.',
			category_type: 'topic',
			confidence: 0.88,
			keywords: ['returns', 'filing', 'compliance'],
			create_time: '20260501-091500'
		},
		childIds: ['finance/tax/federal-corporate'],
		summaryIds: ['sum-tax-1', 'sum-tax-2'],
		expanded: true
	},
	{
		id: 'finance/tax/federal-corporate',
		categoryPath: 'finance/tax/federal-corporate',
		label: 'Federal Corporate',
		metadata: {
			desc: 'Corporate federal filing guidance and evidence.',
			category_type: 'cluster',
			confidence: 0.82,
			keywords: ['federal', 'corporate', 'filing'],
			create_time: '20260501-093000'
		},
		childIds: [],
		summaryIds: ['sum-fed-1'],
		expanded: false
	},
	{
		id: 'finance/payroll',
		categoryPath: 'finance/payroll',
		label: 'Payroll',
		metadata: {
			desc: 'Payroll process notes and compliance items.',
			category_type: 'topic',
			confidence: 0.76,
			keywords: ['payroll', 'w2', 'withholding'],
			create_time: '20260501-094500'
		},
		childIds: [],
		summaryIds: ['sum-pay-1'],
		expanded: false
	},
	{
		id: 'research',
		categoryPath: 'research',
		label: 'Research',
		metadata: {
			desc: 'Long-form research and standards material.',
			category_type: 'domain',
			confidence: 0.9,
			keywords: ['research', 'standard', 'analysis'],
			create_time: '20260501-100000'
		},
		childIds: ['research/standards/engineering/high-voltage/legacy-archive'],
		summaryIds: [],
		expanded: true
	},
	{
		id: 'research/standards/engineering/high-voltage/legacy-archive',
		categoryPath: 'research/standards/engineering/high-voltage/legacy-archive',
		label: 'Legacy Archive',
		metadata: {
			desc: 'Deep archival standards with intentionally long path names.',
			category_type: 'cluster',
			confidence: 0.67,
			keywords: ['archive', 'standards', 'legacy'],
			create_time: '20260501-102000'
		},
		childIds: [],
		summaryIds: ['sum-legacy-1'],
		expanded: false
	}
];

export const MOCK_CATEGORY_SUMMARIES: Record<string, SummaryRecordCard[]> = {
	'finance/tax': [
		{
			id: 'sum-tax-1',
			pdfFileName: '2025-federal-return-overview.pdf',
			keywords: ['filing', 'deductions', 'evidence'],
			summaryText:
				'Captures the filing timeline, document dependencies, and missing support for the current federal return package.',
			inputId: 101,
			page: 4
		},
		{
			id: 'sum-tax-2',
			pdfFileName: 'state-apportionment-notes.pdf',
			keywords: ['state', 'apportionment', 'allocation'],
			summaryText:
				'Summarizes allocation assumptions, inconsistent state language, and unresolved follow-up items.',
			inputId: 102,
			page: 11
		}
	],
	'finance/tax/federal-corporate': [
		{
			id: 'sum-fed-1',
			pdfFileName: 'corporate-federal-checklist.pdf',
			keywords: ['corporate', 'checklist', 'controls'],
			summaryText:
				'Lists the control checkpoints that need review before final corporate filing sign-off.',
			inputId: 103,
			page: 7
		}
	],
	'finance/payroll': [
		{
			id: 'sum-pay-1',
			pdfFileName: 'payroll-quarterly-review.pdf',
			keywords: ['payroll', 'quarterly', 'variance'],
			summaryText:
				'Highlights payroll variance drivers and delayed withholding reconciliations from the quarterly review.',
			inputId: 104,
			page: 3
		}
	],
	'research/standards/engineering/high-voltage/legacy-archive': [
		{
			id: 'sum-legacy-1',
			pdfFileName: 'hv-standard-legacy-appendix.pdf',
			keywords: ['high-voltage', 'legacy', 'appendix'],
			summaryText:
				'Collects archived appendix notes and exceptions that still affect downstream engineering interpretation.',
			inputId: 105,
			page: 19
		}
	]
};

export const MOCK_SUMMARY_TREE_RECORDS: SummaryTreeRecord[] = [
	{
		id: 101,
		title: '2025 Federal Return Overview',
		fileName: '2025-federal-return-overview.pdf',
		docType: 'pdf',
		docNo: 'TAX-2025-001',
		parserName: 'mineru',
		procStatus: 'success',
		createTime: '2026-04-28 09:15:00',
		modifyTime: '2026-04-29 10:20:00',
		summaries: [
			{
				recordId: 101,
				id: 'sum-tax-1',
				pdfFileName: '2025-federal-return-overview.pdf',
				keywords: ['filing', 'deductions', 'evidence'],
				summaryText:
					'Federal filing package summary with evidence gaps and dependency notes.',
				inputId: 101,
				page: 4
			}
		]
	},
	{
		id: 105,
		title: 'HV Standard Legacy Appendix',
		fileName: 'hv-standard-legacy-appendix.pdf',
		docType: 'pdf',
		docNo: 'STD-HV-077',
		parserName: 'docling',
		procStatus: 'success',
		createTime: '2026-04-22 14:00:00',
		modifyTime: '2026-04-25 16:45:00',
		summaries: [
			{
				recordId: 105,
				id: 'sum-legacy-1',
				pdfFileName: 'hv-standard-legacy-appendix.pdf',
				keywords: ['high-voltage', 'legacy', 'appendix'],
				summaryText:
					'Archived appendix guidance affecting long-tail engineering interpretations.',
				inputId: 105,
				page: 19
			}
		]
	}
];

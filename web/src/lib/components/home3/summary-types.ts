export type SummaryCategoryMetadata = {
	desc: string;
	category_type: string;
	confidence: number;
	keywords: string[];
	create_time: string;
};

export type SummaryPdfTarget = {
	inputId: number;
	page: number;
	summaryId: string;
};

export type SummaryRecordTarget = {
	page: number;
	coords: number[];
};

export type SummaryRecordCard = {
	id: string;
	pdfFileName: string;
	keywords: string[];
	summaryText: string;
	inputId: number;
	page: number;
	coords: number[];
	targets: SummaryRecordTarget[];
};

export type SummaryCategoryNode = {
	id: string;
	categoryPath: string;
	label: string;
	metadata: SummaryCategoryMetadata;
	childIds: string[];
	summaryIds: string[];
	expanded?: boolean;
};

export type SummaryCategoryTab = {
	id: string;
	label: string;
	categoryPath: string | null;
	closable: boolean;
};

export type SummaryTreeSummary = SummaryRecordCard & {
	recordId: number;
};

export type SummaryTreeRecord = {
	id: number;
	title: string;
	fileName: string;
	docType: string;
	docNo: string;
	parserName: string;
	procStatus: string;
	createTime: string;
	modifyTime: string;
	summaries: SummaryTreeSummary[];
};

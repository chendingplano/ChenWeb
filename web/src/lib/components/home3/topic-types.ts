export type TopicCategoryMetadata = {
	desc: string;
	confidence: number;
	keywords: string[];
	create_time: string;
};

export type TopicPdfTarget = {
	inputId: number;
	page: number;
	topicId: string;
};

export type TopicRecordTarget = {
	page: number;
	coords: number[];
};

export type TopicCard = {
	id: string;
	topicName?: string;
	pdfFileName: string;
	topicKeywords: string[];
	topicKeywordsEn?: string[];
	topicText: string;
	topicDescEn?: string;
	topicType: string;
	categoryPaths?: string[];
	categoryPathsEn?: string[];
	sourceLineSpecs?: string[];
	inputId: number;
	page: number;
	coords: number[];
	targets: TopicRecordTarget[];
};

export type TopicCategoryNode = {
	id: string;
	categoryPath: string;
	label: string;
	metadata: TopicCategoryMetadata;
	childIds: string[];
	topicIds: string[];
	hasTopicsFile: boolean;
	expanded?: boolean;
};

export type TopicCategoryTab = {
	id: string;
	label: string;
	categoryPath: string | null;
	closable: boolean;
};

export type TopicTreeTopic = TopicCard & {
	recordId: number;
};

export type TopicTreeRecord = {
	id: number;
	title: string;
	fileName: string;
	docType: string;
	docNo: string;
	parserName: string;
	procStatus: string;
	createTime: string;
	modifyTime: string;
	topics: TopicTreeTopic[];
};

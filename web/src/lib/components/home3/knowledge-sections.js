export const KNOWLEDGE_UNDER_CONSTRUCTION_SECTIONS = [
	{
		id: 'kb-references',
		label: 'References',
		description: 'Reference extraction workspace'
	},
	{
		id: 'kb-formulas',
		label: 'Formulas',
		description: 'Formula extraction workspace'
	},
	{
		id: 'kb-tables',
		label: 'Tables',
		description: 'Table extraction workspace'
	},
	{
		id: 'kb-quotations',
		label: 'Quotations',
		description: 'Quotation extraction workspace'
	},
	{
		id: 'kb-case-studies',
		label: 'Case Studies',
		description: 'Case study knowledge workspace'
	},
	{
		id: 'kb-workflow',
		label: 'Workflow',
		description: 'Knowledge workflow workspace'
	},
	{
		id: 'kb-product-parts',
		label: 'Product and Parts',
		description: 'Product and part knowledge'
	}
];

/** @param {string} sectionId */
export function isUnderConstructionKnowledgeSection(sectionId) {
	return KNOWLEDGE_UNDER_CONSTRUCTION_SECTIONS.some((section) => section.id === sectionId);
}

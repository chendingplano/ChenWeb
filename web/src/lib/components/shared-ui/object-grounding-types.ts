/** Host adapter contract: reviewers and artifact pages own their save operation. */
export type GroundingItem = {
	id: string;
	label: string;
	name: string;
	nameEN?: string;
	nameZH?: string;
	kind: string;
	aliases?: string[];
	description?: string;
	details?: { label: string; value: string }[];
};

export type GroundingInput = {
	object_id?: string;
	create?: boolean;
	name?: string;
	name_en?: string;
	name_zh?: string;
	aliases?: string[];
	description?: string;
};

export type KnowledgeStoreUser = {
	id?: string;
	name?: string;
	email?: string;
};

export type KnowledgeStoreUserOption = {
	value: string;
	label: string;
};

export function buildKnowledgeStoreUserOptions(
	users: KnowledgeStoreUser[] = []
): KnowledgeStoreUserOption[] {
	return users
		.map((user) => {
			const value = user.id?.trim() ?? '';
			if (!value) return null;
			const name = user.name?.trim() ?? '';
			const email = user.email?.trim() ?? '';
			return {
				value,
				label: name && email ? `${name} (${email})` : name || email || value
			};
		})
		.filter((option): option is KnowledgeStoreUserOption => option !== null)
		.sort((a, b) => a.label.localeCompare(b.label));
}

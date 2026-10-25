export type ReusableModule = {
	id: number;
	module_id: string;
	module_name: string;
	module_type: 'frontend' | 'backend' | 'script';
	prog_language: string;
	description: string;
	files: string[];
	version: string;
	config_json: Record<string, unknown>;
	status: 'active' | 'under-dev' | 'retired';
	doc_path: string;
	created_by: string;
	updated_by: string;
	created_at: string;
	updated_at: string;
};

export function filterModules(modules: ReusableModule[], query: string): ReusableModule[] {
	const term = query.trim().toLocaleLowerCase();
	if (!term) return modules;
	return modules.filter((module) => [
		module.module_id, module.module_name, module.description, module.module_type,
		module.prog_language, module.status, module.version, module.doc_path,
		...module.files, JSON.stringify(module.config_json)
	].join('\n').toLocaleLowerCase().includes(term));
}

export type CodeMonitorRange =
	| 'today'
	| 'yesterday'
	| 'last_7_days'
	| 'last_month'
	| 'last_6_months'
	| 'last_12_months';

export type CodeMonitorRow = {
	collection_date: string;
	total_lines: number;
	total_lines_delta: number;
	frontend_files: number;
	frontend_files_delta: number;
	backend_files: number;
	backend_files_delta: number;
};

export type CodeMonitorProject = {
	project_path: string;
	rows: CodeMonitorRow[];
};

export type CodeMonitorResponse = { projects: CodeMonitorProject[] };

export type LatestCodeMonitorResult =
	| { status: 'success'; response: CodeMonitorResponse }
	| { status: 'error'; error: unknown }
	| { status: 'stale' };

export function createLatestCodeMonitorLoader(
	loadRange: (range: CodeMonitorRange) => Promise<CodeMonitorResponse> = getCodeMonitor
) {
	let requestSequence = 0;
	return {
		async load(range: CodeMonitorRange): Promise<LatestCodeMonitorResult> {
			const sequence = ++requestSequence;
			try {
				const response = await loadRange(range);
				return sequence === requestSequence ? { status: 'success', response } : { status: 'stale' };
			} catch (error) {
				return sequence === requestSequence ? { status: 'error', error } : { status: 'stale' };
			}
		}
	};
}

export async function getCodeMonitor(range: CodeMonitorRange): Promise<CodeMonitorResponse> {
	const response = await fetch(`/api/v1/code-monitor?range=${range}`, {
		credentials: 'same-origin',
		cache: 'no-store'
	});
	if (!response.ok) throw new Error(`Request failed (${response.status})`);
	return (await response.json()) as CodeMonitorResponse;
}

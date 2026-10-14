import { m } from '$lib/paraglide/messages.js';
import {
	listGoldOptions,
	type GoldKind,
	type GoldMetricOption,
	type KbInputRecord,
	type ListKbInputsParams
} from '$lib/services/kbService';

// Skill-version and model filters shared by the gold benchmark views (Gold
// Metrics, Reference Benchmark). It loads the (version, model) options of one
// gold set, defaults to the newest version (or All, when allowed) across all models, and gives the
// record browser the params that list one entry per benchmark run.
export class GoldRunFilter {
	readonly kind: GoldKind;
	// When set, an empty skillVersion means "All" versions and is the default.
	readonly allowAllVersions: boolean;
	versions = $state<string[]>([]);
	options = $state<GoldMetricOption[]>([]);
	skillVersion = $state('');
	modelName = $state('');
	loading = $state(true);
	error = $state('');
	modelNames = $derived([
		...new Set(
			this.options
				.filter((o) => !this.skillVersion || o.skill_version === this.skillVersion)
				.map((o) => o.model_name)
		)
	]);

	constructor(kind: GoldKind, opts: { allowAllVersions?: boolean } = {}) {
		this.kind = kind;
		this.allowAllVersions = opts.allowAllVersions ?? false;
	}

	async load() {
		this.loading = true;
		this.error = '';
		try {
			const res = await listGoldOptions(this.kind);
			this.versions = res.skill_versions ?? [];
			this.options = res.options ?? [];
			if (!this.versions.includes(this.skillVersion)) {
				this.skillVersion = this.allowAllVersions ? '' : (this.versions[0] ?? '');
			}
		} catch (err) {
			this.error = err instanceof Error ? err.message : m.gold_run_filters_options_failed();
		} finally {
			this.loading = false;
		}
	}

	setSkillVersion(version: string) {
		this.skillVersion = version;
		this.modelName = '';
	}

	setModelName(name: string) {
		this.modelName = name;
	}

	// Record-browser params: one list entry per benchmark run of this gold set.
	get listParams(): Partial<ListKbInputsParams> {
		return {
			hasGoldMetrics: true,
			goldBenchmarkEntries: true,
			goldKind: this.kind,
			goldSkillVersion: this.skillVersion,
			goldModelName: this.modelName
		};
	}

	// Shown instead of the record list until the options are loaded.
	get pausedMessage(): string {
		if (!this.loading && (this.skillVersion || this.allowAllVersions)) return '';
		return this.error || (this.loading ? m.kb_input_record_browser_loading_records() : m.metric_mgmt_no_records_yet());
	}
}

// Badge text for each benchmark run attached to a record-browser entry.
export function goldRunBadges(record: KbInputRecord): string[] {
	return (record.gold_metric_runs ?? []).map((run) =>
		m.gold_run_filters_benchmark_run({ version: run.skill_version, model: run.model_name, run: run.run_id })
	);
}

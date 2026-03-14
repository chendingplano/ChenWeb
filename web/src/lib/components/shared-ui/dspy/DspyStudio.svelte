<script lang="ts">
	// DSPy Prompt Studio
	// Lets users declare what they want to achieve, then builds DSPy-optimized prompts
	// following the DSPy workflow: Signature → Module → Examples → Optimizer → Optimize

	type SignatureField = {
		name: string;
		desc: string;
		type: 'str' | 'int' | 'float' | 'bool' | 'list' | 'dict';
	};

	type TrainingExample = {
		id: string;
		inputs: Record<string, string>;
		outputs: Record<string, string>;
	};

	type DspyPrompt = {
		prompt_id: string;
		prompt_name: string;
		prompt_desc: string;
		task_type: string;
		signature_inputs: SignatureField[];
		signature_outputs: SignatureField[];
		signature_docstring: string;
		module_type: string;
		examples: TrainingExample[];
		optimizer: string;
		optimizer_config: Record<string, unknown>;
		optimized_instructions: string;
		optimized_examples: TrainingExample[];
		status: string;
		created_at: string;
		updated_at: string;
	};

	// ─── State ───────────────────────────────────────────────────────────────
	let activeTab = $state<'create' | 'library' | 'references'>('create');

	// Wizard step (1-6)
	let wizardStep = $state(1);

	// Create form state
	let form = $state({
		prompt_name: '',
		prompt_desc: '',
		task_type: 'generation',
		signature_docstring: '',
		signature_inputs: [{ name: 'question', desc: 'The question to answer', type: 'str' }] as SignatureField[],
		signature_outputs: [{ name: 'answer', desc: 'The generated answer', type: 'str' }] as SignatureField[],
		module_type: 'ChainOfThought',
		examples: [] as TrainingExample[],
		optimizer: 'BootstrapFewShot',
		optimizer_config: { max_bootstrapped_demos: 4, max_labeled_demos: 16 }
	});

	// New example being added
	let newExampleInputs = $state<Record<string, string>>({});
	let newExampleOutputs = $state<Record<string, string>>({});

	// Saved prompts library
	let savedPrompts = $state<DspyPrompt[]>([]);
	let isLoadingPrompts = $state(false);
	let isOptimizing = $state(false);
	let optimizeResult = $state('');

	// Edit modal
	let editingPrompt = $state<DspyPrompt | null>(null);
	let showEditModal = $state(false);

	// View modal
	let viewingPrompt = $state<DspyPrompt | null>(null);
	let showViewModal = $state(false);

	// Delete confirm
	let deletingPromptId = $state<string | null>(null);
	let showDeleteConfirm = $state(false);

	// Notification
	let notification = $state<{ type: 'success' | 'error'; msg: string } | null>(null);

	// ─── Helpers ─────────────────────────────────────────────────────────────
	function showNotif(type: 'success' | 'error', msg: string) {
		notification = { type, msg };
		setTimeout(() => (notification = null), 3500);
	}

	function resetExampleInputs() {
		newExampleInputs = Object.fromEntries(form.signature_inputs.map((f) => [f.name, '']));
		newExampleOutputs = Object.fromEntries(form.signature_outputs.map((f) => [f.name, '']));
	}

	function addSignatureField(side: 'inputs' | 'outputs') {
		const field: SignatureField = { name: '', desc: '', type: 'str' };
		if (side === 'inputs') form.signature_inputs = [...form.signature_inputs, field];
		else form.signature_outputs = [...form.signature_outputs, field];
	}

	function removeSignatureField(side: 'inputs' | 'outputs', idx: number) {
		if (side === 'inputs')
			form.signature_inputs = form.signature_inputs.filter((_, i) => i !== idx);
		else form.signature_outputs = form.signature_outputs.filter((_, i) => i !== idx);
	}

	function addExample() {
		const hasInputs = form.signature_inputs.every((f) => newExampleInputs[f.name]?.trim());
		const hasOutputs = form.signature_outputs.every((f) => newExampleOutputs[f.name]?.trim());
		if (!hasInputs || !hasOutputs) {
			showNotif('error', 'Fill in all input and output fields for the example.');
			return;
		}
		form.examples = [
			...form.examples,
			{
				id: crypto.randomUUID(),
				inputs: { ...newExampleInputs },
				outputs: { ...newExampleOutputs }
			}
		];
		resetExampleInputs();
	}

	function removeExample(id: string) {
		form.examples = form.examples.filter((e) => e.id !== id);
	}

	// ─── API calls ────────────────────────────────────────────────────────────
	async function fetchPrompts() {
		isLoadingPrompts = true;
		try {
			const res = await fetch('/api/v1/dspy/prompts');
			if (!res.ok) throw new Error('Failed to load prompts');
			const data = await res.json();
			savedPrompts = data.results ?? [];
		} catch (e) {
			showNotif('error', 'Could not load saved prompts.');
		} finally {
			isLoadingPrompts = false;
		}
	}

	async function savePrompt() {
		try {
			const body = {
				prompt_name: form.prompt_name,
				prompt_desc: form.prompt_desc,
				task_type: form.task_type,
				signature_inputs: JSON.stringify(form.signature_inputs),
				signature_outputs: JSON.stringify(form.signature_outputs),
				signature_docstring: form.signature_docstring,
				module_type: form.module_type,
				examples: JSON.stringify(form.examples),
				optimizer: form.optimizer,
				optimizer_config: JSON.stringify(form.optimizer_config),
				status: 'draft'
			};
			const res = await fetch('/api/v1/dspy/prompts', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(body)
			});
			if (!res.ok) throw new Error('Failed to save');
			showNotif('success', 'Prompt saved successfully!');
			resetForm();
			activeTab = 'library';
			fetchPrompts();
		} catch {
			showNotif('error', 'Failed to save prompt. Please try again.');
		}
	}

	async function runOptimize() {
		if (!form.prompt_name) {
			showNotif('error', 'Please complete the form first (Step 1).');
			return;
		}
		isOptimizing = true;
		optimizeResult = '';
		try {
			const res = await fetch('/api/v1/dspy/optimize', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					prompt_name: form.prompt_name,
					module_type: form.module_type,
					optimizer: form.optimizer,
					examples: form.examples
				})
			});
			if (!res.ok) throw new Error('Optimize failed');
			const data = await res.json();
			optimizeResult = data.optimized_instructions ?? 'Optimization complete.';
			showNotif('success', 'Optimization complete!');
		} catch {
			showNotif('error', 'Optimization failed. This is a stub endpoint.');
			optimizeResult = '(Stub) Optimization would run here with the configured optimizer and examples.';
		} finally {
			isOptimizing = false;
		}
	}

	async function deletePrompt(id: string) {
		try {
			const res = await fetch(`/api/v1/dspy/prompts/${id}`, { method: 'DELETE' });
			if (!res.ok) throw new Error('Delete failed');
			savedPrompts = savedPrompts.filter((p) => p.prompt_id !== id);
			showNotif('success', 'Prompt deleted.');
		} catch {
			showNotif('error', 'Failed to delete prompt.');
		} finally {
			showDeleteConfirm = false;
			deletingPromptId = null;
		}
	}

	async function updatePrompt() {
		if (!editingPrompt) return;
		try {
			const res = await fetch(`/api/v1/dspy/prompts/${editingPrompt.prompt_id}`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(editingPrompt)
			});
			if (!res.ok) throw new Error('Update failed');
			showNotif('success', 'Prompt updated.');
			showEditModal = false;
			fetchPrompts();
		} catch {
			showNotif('error', 'Failed to update prompt.');
		}
	}

	function resetForm() {
		form = {
			prompt_name: '',
			prompt_desc: '',
			task_type: 'generation',
			signature_docstring: '',
			signature_inputs: [{ name: 'question', desc: 'The question to answer', type: 'str' }],
			signature_outputs: [{ name: 'answer', desc: 'The generated answer', type: 'str' }],
			module_type: 'ChainOfThought',
			examples: [],
			optimizer: 'BootstrapFewShot',
			optimizer_config: { max_bootstrapped_demos: 4, max_labeled_demos: 16 }
		};
		wizardStep = 1;
		optimizeResult = '';
	}

	// Load prompts when switching to library tab
	function switchTab(tab: 'create' | 'library') {
		activeTab = tab;
		if (tab === 'library') fetchPrompts();
	}

	// Step validation
	function canProceedStep1() {
		return form.prompt_name.trim().length > 0;
	}
	function canProceedStep2() {
		return (
			form.signature_inputs.every((f) => f.name.trim()) &&
			form.signature_outputs.every((f) => f.name.trim())
		);
	}

	// DSPy config display
	const taskTypes = [
		{ value: 'generation', label: 'Text Generation' },
		{ value: 'classification', label: 'Classification' },
		{ value: 'extraction', label: 'Information Extraction' },
		{ value: 'summarization', label: 'Summarization' },
		{ value: 'qa', label: 'Question Answering' },
		{ value: 'code', label: 'Code Generation' },
		{ value: 'reasoning', label: 'Reasoning' },
		{ value: 'custom', label: 'Custom' }
	];

	const moduleTypes = [
		{ value: 'Predict', label: 'Predict', desc: 'Basic LLM call with your signature' },
		{
			value: 'ChainOfThought',
			label: 'ChainOfThought',
			desc: 'Adds step-by-step reasoning before the answer'
		},
		{
			value: 'ReAct',
			label: 'ReAct',
			desc: 'Interleaves reasoning with tool-use actions'
		},
		{
			value: 'ProgramOfThought',
			label: 'ProgramOfThought',
			desc: 'Generates code to solve the task'
		},
		{
			value: 'MultiChainComparison',
			label: 'MultiChainComparison',
			desc: 'Runs multiple CoT chains and picks the best'
		}
	];

	const optimizers = [
		{
			value: 'BootstrapFewShot',
			label: 'BootstrapFewShot',
			desc: 'Generates few-shot examples from training data'
		},
		{
			value: 'MIPROv2',
			label: 'MIPROv2',
			desc: 'Multi-prompt instruction proposal and optimization'
		},
		{
			value: 'BayesianSignatureOptimizer',
			label: 'BayesianSignatureOptimizer',
			desc: 'Optimizes field descriptions in the signature'
		},
		{
			value: 'COPRO',
			label: 'COPRO',
			desc: 'Coordinate ascent for prompt optimization'
		},
		{ value: 'Ensemble', label: 'Ensemble', desc: 'Combines multiple programs for robustness' }
	];

	const fieldTypes = ['str', 'int', 'float', 'bool', 'list', 'dict'];

	const stepLabels = [
		'Define Task',
		'Signature',
		'Module',
		'Examples',
		'Optimizer',
		'Review & Save'
	];

	// Build signature preview string
	function signaturePreview(): string {
		const ins = form.signature_inputs.map((f) => f.name || '?').join(', ');
		const outs = form.signature_outputs.map((f) => f.name || '?').join(', ');
		return `${ins} -> ${outs}`;
	}
</script>

<div class="flex h-full flex-col overflow-hidden bg-background">
	<!-- Header -->
	<div
		class="flex shrink-0 items-center justify-between border-b bg-gradient-to-r from-violet-600 via-indigo-600 to-blue-600 px-6 py-4 text-white"
	>
		<div>
			<h2 class="text-xl font-bold tracking-tight">DSPy Prompt Studio</h2>
			<p class="mt-0.5 text-sm text-indigo-100">
				Declare intent, not prompts — DSPy handles the optimization
			</p>
		</div>
		<div class="flex items-center gap-2 rounded-full bg-white/15 px-3 py-1 text-xs font-medium backdrop-blur-sm">
			<span class="h-1.5 w-1.5 rounded-full bg-emerald-400"></span>
			DSPy Framework
		</div>
	</div>

	<!-- Tabs -->
	<div class="flex shrink-0 gap-1 border-b bg-muted/30 px-6 pt-3">
		<button
			class="rounded-t-lg px-5 py-2 text-sm font-medium transition-colors cursor-pointer {activeTab === 'create'
				? 'border border-b-background bg-background text-foreground shadow-sm'
				: 'text-muted-foreground hover:text-foreground'}"
			onclick={() => switchTab('create')}
		>
			Create Prompt
		</button>
		<button
			class="rounded-t-lg px-5 py-2 text-sm font-medium transition-colors cursor-pointer {activeTab === 'library'
				? 'border border-b-background bg-background text-foreground shadow-sm'
				: 'text-muted-foreground hover:text-foreground'}"
			onclick={() => switchTab('library')}
		>
			Prompt Library
		</button>
		<button
			class="rounded-t-lg px-5 py-2 text-sm font-medium transition-colors cursor-pointer {activeTab === 'references'
				? 'border border-b-background bg-background text-foreground shadow-sm'
				: 'text-muted-foreground hover:text-foreground'}"
			onclick={() => switchTab('references')}
		>
			References
		</button>
	</div>

	<!-- Notification -->
	{#if notification}
		<div
			class="mx-6 mt-3 shrink-0 rounded-lg border px-4 py-3 text-sm font-medium {notification.type === 'success'
				? 'border-emerald-200 bg-emerald-50 text-emerald-800'
				: 'border-red-200 bg-red-50 text-red-800'}"
		>
			{notification.msg}
		</div>
	{/if}

	<!-- ═══════════════════════════════════════════════════════════════════════
	     CREATE TAB — Wizard
	═══════════════════════════════════════════════════════════════════════ -->
	{#if activeTab === 'create'}
		<div class="flex flex-1 overflow-hidden">
			<!-- Sidebar stepper -->
			<aside class="flex w-52 shrink-0 flex-col gap-1 border-r bg-muted/20 p-4">
				{#each stepLabels as label, i}
					{@const step = i + 1}
					<button
						class="flex items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm transition-colors cursor-pointer {wizardStep === step
							? 'bg-primary text-primary-foreground font-semibold shadow-sm'
							: step < wizardStep
								? 'text-emerald-700 hover:bg-emerald-50'
								: 'text-muted-foreground hover:bg-muted'}"
						onclick={() => (wizardStep = step)}
					>
						<span
							class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-bold {wizardStep === step
								? 'bg-white/25'
								: step < wizardStep
									? 'bg-emerald-100 text-emerald-700'
									: 'bg-muted text-muted-foreground'}"
						>
							{step < wizardStep ? '✓' : step}
						</span>
						{label}
					</button>
				{/each}

				<!-- DSPy concept pill -->
				<div class="mt-auto rounded-lg border bg-card p-3 text-xs text-muted-foreground">
					<div class="mb-1 font-semibold text-foreground">How DSPy works</div>
					<ol class="space-y-1 list-decimal list-inside">
						<li>Define your task & signature</li>
						<li>Choose a module (predictor)</li>
						<li>Add training examples</li>
						<li>Pick an optimizer</li>
						<li>DSPy optimizes the prompt</li>
					</ol>
				</div>
			</aside>

			<!-- Wizard content -->
			<main class="flex-1 overflow-y-auto p-6">
				<!-- ── Step 1: Define Task ────────────────────────────────── -->
				{#if wizardStep === 1}
					<h3 class="mb-1 text-lg font-semibold">Step 1 — Define Your Task</h3>
					<p class="mb-6 text-sm text-muted-foreground">
						Describe what you want the LLM to do. Be specific about the goal.
					</p>
					<div class="max-w-xl space-y-4">
						<div>
							<label class="mb-1.5 block text-sm font-medium" for="prompt-name">
								Prompt Name <span class="text-red-500">*</span>
							</label>
							<input
								id="prompt-name"
								type="text"
								class="w-full rounded-lg border bg-background px-3 py-2 text-sm outline-none ring-offset-background focus:ring-2 focus:ring-primary"
								placeholder="e.g. Customer Support QA"
								bind:value={form.prompt_name}
							/>
						</div>
						<div>
							<label class="mb-1.5 block text-sm font-medium" for="prompt-desc">
								Description
							</label>
							<textarea
								id="prompt-desc"
								rows="3"
								class="w-full rounded-lg border bg-background px-3 py-2 text-sm outline-none ring-offset-background focus:ring-2 focus:ring-primary"
								placeholder="What problem does this prompt solve?"
								bind:value={form.prompt_desc}
							></textarea>
						</div>
						<div>
							<label class="mb-1.5 block text-sm font-medium" for="task-type">Task Type</label>
							<select
								id="task-type"
								class="w-full rounded-lg border bg-background px-3 py-2 text-sm outline-none ring-offset-background focus:ring-2 focus:ring-primary cursor-pointer"
								bind:value={form.task_type}
							>
								{#each taskTypes as t}
									<option value={t.value}>{t.label}</option>
								{/each}
							</select>
						</div>
					</div>
					<div class="mt-6 flex gap-3">
						<button
							class="rounded-lg bg-primary px-5 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50 cursor-pointer"
							disabled={!canProceedStep1()}
							onclick={() => (wizardStep = 2)}
						>
							Next: Signature →
						</button>
					</div>

				<!-- ── Step 2: Signature ────────────────────────────────── -->
				{:else if wizardStep === 2}
					<h3 class="mb-1 text-lg font-semibold">Step 2 — Define Signature</h3>
					<p class="mb-2 text-sm text-muted-foreground">
						A DSPy signature declares the inputs and outputs for your module — like a typed function
						signature for your LLM.
					</p>
					<div class="mb-4 rounded-lg border bg-indigo-50 px-4 py-2 text-sm font-mono text-indigo-700">
						{signaturePreview()}
					</div>

					<div class="mb-6">
						<label class="mb-1.5 block text-sm font-medium" for="sig-docstring">
							Docstring (task instructions for the LLM)
						</label>
						<textarea
							id="sig-docstring"
							rows="2"
							class="w-full rounded-lg border bg-background px-3 py-2 text-sm outline-none ring-offset-background focus:ring-2 focus:ring-primary"
							placeholder="e.g. Given a question, produce a detailed and accurate answer."
							bind:value={form.signature_docstring}
						></textarea>
					</div>

					<div class="grid gap-6 md:grid-cols-2">
						<!-- Inputs -->
						<div>
							<div class="mb-2 flex items-center justify-between">
								<span class="text-sm font-semibold text-blue-700">Input Fields</span>
								<button
									class="rounded-md border px-2 py-1 text-xs transition-colors hover:bg-muted cursor-pointer"
									onclick={() => addSignatureField('inputs')}
								>+ Add Input</button>
							</div>
							{#each form.signature_inputs as field, i}
								<div class="mb-2 rounded-lg border bg-card p-3 shadow-sm">
									<div class="mb-2 flex items-center gap-2">
										<input
											type="text"
											class="flex-1 rounded border bg-background px-2 py-1 text-sm outline-none focus:ring-2 focus:ring-primary"
											placeholder="Field name"
											bind:value={field.name}
										/>
										<select
											class="rounded border bg-background px-2 py-1 text-xs outline-none cursor-pointer"
											bind:value={field.type}
										>
											{#each fieldTypes as t}
												<option value={t}>{t}</option>
											{/each}
										</select>
										{#if form.signature_inputs.length > 1}
											<button
												class="text-red-400 hover:text-red-600 transition-colors cursor-pointer"
												onclick={() => removeSignatureField('inputs', i)}>✕</button
											>
										{/if}
									</div>
									<input
										type="text"
										class="w-full rounded border bg-background px-2 py-1 text-xs outline-none focus:ring-2 focus:ring-primary"
										placeholder="Description (e.g. The user's question)"
										bind:value={field.desc}
									/>
								</div>
							{/each}
						</div>

						<!-- Outputs -->
						<div>
							<div class="mb-2 flex items-center justify-between">
								<span class="text-sm font-semibold text-emerald-700">Output Fields</span>
								<button
									class="rounded-md border px-2 py-1 text-xs transition-colors hover:bg-muted cursor-pointer"
									onclick={() => addSignatureField('outputs')}
								>+ Add Output</button>
							</div>
							{#each form.signature_outputs as field, i}
								<div class="mb-2 rounded-lg border bg-card p-3 shadow-sm">
									<div class="mb-2 flex items-center gap-2">
										<input
											type="text"
											class="flex-1 rounded border bg-background px-2 py-1 text-sm outline-none focus:ring-2 focus:ring-primary"
											placeholder="Field name"
											bind:value={field.name}
										/>
										<select
											class="rounded border bg-background px-2 py-1 text-xs outline-none cursor-pointer"
											bind:value={field.type}
										>
											{#each fieldTypes as t}
												<option value={t}>{t}</option>
											{/each}
										</select>
										{#if form.signature_outputs.length > 1}
											<button
												class="text-red-400 hover:text-red-600 transition-colors cursor-pointer"
												onclick={() => removeSignatureField('outputs', i)}>✕</button
											>
										{/if}
									</div>
									<input
										type="text"
										class="w-full rounded border bg-background px-2 py-1 text-xs outline-none focus:ring-2 focus:ring-primary"
										placeholder="Description (e.g. Detailed accurate answer)"
										bind:value={field.desc}
									/>
								</div>
							{/each}
						</div>
					</div>

					<div class="mt-6 flex gap-3">
						<button
							class="rounded-lg border px-5 py-2 text-sm font-medium transition-colors hover:bg-muted cursor-pointer"
							onclick={() => (wizardStep = 1)}
						>← Back</button>
						<button
							class="rounded-lg bg-primary px-5 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50 cursor-pointer"
							disabled={!canProceedStep2()}
							onclick={() => (wizardStep = 3)}
						>Next: Module →</button>
					</div>

				<!-- ── Step 3: Module ────────────────────────────────── -->
				{:else if wizardStep === 3}
					<h3 class="mb-1 text-lg font-semibold">Step 3 — Choose Module</h3>
					<p class="mb-6 text-sm text-muted-foreground">
						DSPy modules wrap your signature with a prompting strategy. Choose how the LLM should
						approach the task.
					</p>
					<div class="max-w-2xl space-y-3">
						{#each moduleTypes as mod}
							<label
								class="flex cursor-pointer items-start gap-4 rounded-xl border p-4 transition-all hover:shadow-sm {form.module_type === mod.value
									? 'border-primary bg-primary/5 shadow-sm'
									: 'bg-card hover:border-primary/40'}"
							>
								<input
									type="radio"
									class="mt-0.5"
									name="module_type"
									value={mod.value}
									bind:group={form.module_type}
								/>
								<div>
									<div class="font-semibold text-sm font-mono">{mod.label}</div>
									<div class="text-xs text-muted-foreground mt-0.5">{mod.desc}</div>
								</div>
							</label>
						{/each}
					</div>
					<div class="mt-6 flex gap-3">
						<button
							class="rounded-lg border px-5 py-2 text-sm font-medium transition-colors hover:bg-muted cursor-pointer"
							onclick={() => (wizardStep = 2)}
						>← Back</button>
						<button
							class="rounded-lg bg-primary px-5 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 cursor-pointer"
							onclick={() => { wizardStep = 4; resetExampleInputs(); }}
						>Next: Examples →</button>
					</div>

				<!-- ── Step 4: Examples ────────────────────────────────── -->
				{:else if wizardStep === 4}
					<h3 class="mb-1 text-lg font-semibold">Step 4 — Training Examples</h3>
					<p class="mb-6 text-sm text-muted-foreground">
						Add input/output demonstrations. The optimizer uses these to generate or select few-shot
						examples for the optimized prompt.
					</p>

					<!-- Add example form -->
					<div class="mb-6 rounded-xl border bg-card p-5 shadow-sm">
						<div class="mb-3 text-sm font-semibold text-muted-foreground">New Example</div>
						<div class="grid gap-4 md:grid-cols-2">
							<div>
								<div class="mb-1.5 text-xs font-semibold uppercase tracking-wide text-blue-600">
									Inputs
								</div>
								{#each form.signature_inputs as field}
									<div class="mb-2">
										<label for="input-{field.name}" class="mb-1 block text-xs text-muted-foreground">{field.name}</label>
										<textarea
											id="input-{field.name}"
											rows="2"
											class="w-full rounded-lg border bg-background px-2 py-1.5 text-sm outline-none focus:ring-2 focus:ring-primary"
											placeholder="Example value…"
											bind:value={newExampleInputs[field.name]}
										></textarea>
									</div>
								{/each}
							</div>
							<div>
								<div class="mb-1.5 text-xs font-semibold uppercase tracking-wide text-emerald-600">
									Expected Outputs
								</div>
								{#each form.signature_outputs as field}
									<div class="mb-2">
										<label for="output-{field.name}" class="mb-1 block text-xs text-muted-foreground">{field.name}</label>
										<textarea
											id="output-{field.name}"
											rows="2"
											class="w-full rounded-lg border bg-background px-2 py-1.5 text-sm outline-none focus:ring-2 focus:ring-primary"
											placeholder="Expected value…"
											bind:value={newExampleOutputs[field.name]}
										></textarea>
									</div>
								{/each}
							</div>
						</div>
						<button
							class="mt-3 rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-indigo-700 cursor-pointer"
							onclick={addExample}
						>+ Add Example</button>
					</div>

					<!-- Existing examples -->
					{#if form.examples.length > 0}
						<div class="space-y-3">
							{#each form.examples as ex, i}
								<div class="rounded-xl border bg-card p-4 shadow-sm">
									<div class="mb-2 flex items-center justify-between">
										<span class="text-xs font-semibold text-muted-foreground">
											Example #{i + 1}
										</span>
										<button
											class="text-xs text-red-500 hover:text-red-700 transition-colors cursor-pointer"
											onclick={() => removeExample(ex.id)}>Remove</button
										>
									</div>
									<div class="grid gap-3 md:grid-cols-2 text-xs">
										<div>
											<div class="mb-1 font-semibold text-blue-600 uppercase tracking-wide">Inputs</div>
											{#each Object.entries(ex.inputs) as [k, v]}
												<div class="mb-1"><span class="font-medium">{k}:</span> {v}</div>
											{/each}
										</div>
										<div>
											<div class="mb-1 font-semibold text-emerald-600 uppercase tracking-wide">Outputs</div>
											{#each Object.entries(ex.outputs) as [k, v]}
												<div class="mb-1"><span class="font-medium">{k}:</span> {v}</div>
											{/each}
										</div>
									</div>
								</div>
							{/each}
						</div>
					{:else}
						<p class="text-sm text-muted-foreground italic">No examples added yet. Examples are optional but strongly recommended for better optimization.</p>
					{/if}

					<div class="mt-6 flex gap-3">
						<button
							class="rounded-lg border px-5 py-2 text-sm font-medium transition-colors hover:bg-muted cursor-pointer"
							onclick={() => (wizardStep = 3)}
						>← Back</button>
						<button
							class="rounded-lg bg-primary px-5 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 cursor-pointer"
							onclick={() => (wizardStep = 5)}
						>Next: Optimizer →</button>
					</div>

				<!-- ── Step 5: Optimizer ────────────────────────────────── -->
				{:else if wizardStep === 5}
					<h3 class="mb-1 text-lg font-semibold">Step 5 — Choose Optimizer</h3>
					<p class="mb-6 text-sm text-muted-foreground">
						DSPy optimizers (teleprompters) automatically improve your prompt using the training
						examples. They find the best instructions and few-shot demonstrations.
					</p>
					<div class="max-w-2xl space-y-3">
						{#each optimizers as opt}
							<label
								class="flex cursor-pointer items-start gap-4 rounded-xl border p-4 transition-all hover:shadow-sm {form.optimizer === opt.value
									? 'border-primary bg-primary/5 shadow-sm'
									: 'bg-card hover:border-primary/40'}"
							>
								<input
									type="radio"
									class="mt-0.5"
									name="optimizer"
									value={opt.value}
									bind:group={form.optimizer}
								/>
								<div>
									<div class="font-semibold text-sm font-mono">{opt.label}</div>
									<div class="text-xs text-muted-foreground mt-0.5">{opt.desc}</div>
								</div>
							</label>
						{/each}
					</div>

					{#if form.optimizer === 'BootstrapFewShot'}
						<div class="mt-4 max-w-sm space-y-3 rounded-xl border bg-card p-4">
							<div class="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
								BootstrapFewShot Settings
							</div>
							<div>
								<label for="max-bootstrapped" class="mb-1 block text-xs font-medium">Max Bootstrapped Demos</label>
								<input
									id="max-bootstrapped"
									type="number"
									min="1"
									max="32"
									class="w-full rounded border bg-background px-2 py-1 text-sm outline-none focus:ring-2 focus:ring-primary"
									bind:value={(form.optimizer_config as any).max_bootstrapped_demos}
								/>
							</div>
							<div>
								<label for="max-labeled" class="mb-1 block text-xs font-medium">Max Labeled Demos</label>
								<input
									id="max-labeled"
									type="number"
									min="1"
									max="64"
									class="w-full rounded border bg-background px-2 py-1 text-sm outline-none focus:ring-2 focus:ring-primary"
									bind:value={(form.optimizer_config as any).max_labeled_demos}
								/>
							</div>
						</div>
					{/if}

					<div class="mt-6 flex gap-3">
						<button
							class="rounded-lg border px-5 py-2 text-sm font-medium transition-colors hover:bg-muted cursor-pointer"
							onclick={() => (wizardStep = 4)}
						>← Back</button>
						<button
							class="rounded-lg bg-primary px-5 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 cursor-pointer"
							onclick={() => (wizardStep = 6)}
						>Next: Review & Save →</button>
					</div>

				<!-- ── Step 6: Review & Save ────────────────────────────── -->
				{:else if wizardStep === 6}
					<h3 class="mb-1 text-lg font-semibold">Step 6 — Review & Save</h3>
					<p class="mb-6 text-sm text-muted-foreground">
						Review your DSPy configuration before saving. You can run optimization now or later.
					</p>
					<div class="max-w-2xl space-y-4">
						<!-- Summary card -->
						<div class="rounded-xl border bg-card p-5 shadow-sm space-y-3">
							<div class="grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
								<div>
									<span class="text-muted-foreground">Name:</span>
									<span class="ml-2 font-semibold">{form.prompt_name}</span>
								</div>
								<div>
									<span class="text-muted-foreground">Task Type:</span>
									<span class="ml-2 font-semibold">{form.task_type}</span>
								</div>
								<div>
									<span class="text-muted-foreground">Module:</span>
									<span class="ml-2 font-mono font-semibold text-indigo-600">{form.module_type}</span>
								</div>
								<div>
									<span class="text-muted-foreground">Optimizer:</span>
									<span class="ml-2 font-mono font-semibold text-violet-600">{form.optimizer}</span>
								</div>
								<div>
									<span class="text-muted-foreground">Examples:</span>
									<span class="ml-2 font-semibold">{form.examples.length}</span>
								</div>
							</div>
							<div>
								<span class="text-sm text-muted-foreground">Signature:</span>
								<div class="mt-1 rounded-md bg-indigo-50 px-3 py-1.5 font-mono text-sm text-indigo-700">
									{signaturePreview()}
								</div>
							</div>
							{#if form.signature_docstring}
								<div>
									<span class="text-sm text-muted-foreground">Docstring:</span>
									<div class="mt-1 text-sm italic text-foreground">{form.signature_docstring}</div>
								</div>
							{/if}
						</div>

						<!-- Optimize button -->
						<div class="rounded-xl border bg-gradient-to-br from-violet-50 to-indigo-50 p-5">
							<div class="mb-2 text-sm font-semibold">Run Optimization</div>
							<p class="mb-3 text-xs text-muted-foreground">
								Trigger the DSPy optimizer to generate optimized instructions and few-shot examples
								from your training data.
							</p>
							<button
								class="rounded-lg bg-violet-600 px-5 py-2 text-sm font-medium text-white transition-colors hover:bg-violet-700 disabled:opacity-60 cursor-pointer"
								disabled={isOptimizing}
								onclick={runOptimize}
							>
								{#if isOptimizing}
									<span class="flex items-center gap-2">
										<span class="h-3 w-3 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
										Optimizing…
									</span>
								{:else}
									Run Optimizer
								{/if}
							</button>
							{#if optimizeResult}
								<div class="mt-3 rounded-lg border border-violet-200 bg-white p-3 text-xs font-mono text-violet-800">
									{optimizeResult}
								</div>
							{/if}
						</div>
					</div>

					<div class="mt-6 flex gap-3">
						<button
							class="rounded-lg border px-5 py-2 text-sm font-medium transition-colors hover:bg-muted cursor-pointer"
							onclick={() => (wizardStep = 5)}
						>← Back</button>
						<button
							class="rounded-lg bg-emerald-600 px-5 py-2 text-sm font-medium text-white transition-colors hover:bg-emerald-700 cursor-pointer"
							onclick={savePrompt}
						>Save to Library</button>
						<button
							class="rounded-lg border px-5 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted cursor-pointer"
							onclick={resetForm}
						>Reset Form</button>
					</div>
				{/if}
			</main>
		</div>
	{/if}

	<!-- ═══════════════════════════════════════════════════════════════════════
	     LIBRARY TAB
	═══════════════════════════════════════════════════════════════════════ -->
	{#if activeTab === 'library'}
		<div class="flex-1 overflow-y-auto p-6">
			<div class="mb-4 flex items-center justify-between">
				<h3 class="text-lg font-semibold">Saved Prompts</h3>
				<button
					class="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 cursor-pointer"
					onclick={() => switchTab('create')}
				>+ New Prompt</button>
			</div>

			{#if isLoadingPrompts}
				<div class="flex items-center gap-3 py-12 text-muted-foreground">
					<span class="h-5 w-5 animate-spin rounded-full border-2 border-primary border-t-transparent"></span>
					Loading prompts…
				</div>
			{:else if savedPrompts.length === 0}
				<div class="rounded-xl border bg-card p-12 text-center">
					<div class="mb-2 text-4xl">📋</div>
					<p class="text-muted-foreground">No prompts saved yet.</p>
					<button
						class="mt-4 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 cursor-pointer"
						onclick={() => switchTab('create')}
					>Create your first prompt</button>
				</div>
			{:else}
				<div class="space-y-3">
					{#each savedPrompts as prompt}
						<div
							class="rounded-xl border bg-card p-5 shadow-sm transition-shadow hover:shadow-md"
						>
							<div class="flex items-start justify-between gap-4">
								<div class="flex-1 min-w-0">
									<div class="flex items-center gap-2 flex-wrap">
										<h4 class="font-semibold truncate">{prompt.prompt_name}</h4>
										<span
											class="rounded-full px-2 py-0.5 text-xs font-medium {prompt.status === 'optimized'
												? 'bg-emerald-100 text-emerald-700'
												: prompt.status === 'optimizing'
													? 'bg-yellow-100 text-yellow-700'
													: 'bg-muted text-muted-foreground'}"
										>
											{prompt.status}
										</span>
										<span class="rounded-full bg-indigo-100 px-2 py-0.5 text-xs text-indigo-700 font-mono">
											{prompt.module_type}
										</span>
									</div>
									{#if prompt.prompt_desc}
										<p class="mt-1 text-sm text-muted-foreground truncate">{prompt.prompt_desc}</p>
									{/if}
									<div class="mt-2 flex flex-wrap gap-2 text-xs text-muted-foreground">
										<span>Task: {prompt.task_type}</span>
										<span>·</span>
										<span>Optimizer: {prompt.optimizer}</span>
										<span>·</span>
										<span>Created: {prompt.created_at ? new Date(prompt.created_at).toLocaleDateString() : '—'}</span>
									</div>
								</div>
								<div class="flex shrink-0 items-center gap-2">
									<button
										class="rounded-md border px-3 py-1.5 text-xs font-medium transition-colors hover:bg-muted cursor-pointer"
										onclick={() => { viewingPrompt = prompt; showViewModal = true; }}
									>View</button>
									<button
										class="rounded-md border px-3 py-1.5 text-xs font-medium transition-colors hover:bg-muted cursor-pointer"
										onclick={() => { editingPrompt = { ...prompt }; showEditModal = true; }}
									>Edit</button>
									<button
										class="rounded-md border border-red-200 px-3 py-1.5 text-xs font-medium text-red-600 transition-colors hover:bg-red-50 cursor-pointer"
										onclick={() => { deletingPromptId = prompt.prompt_id; showDeleteConfirm = true; }}
									>Delete</button>
								</div>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}

	<!-- ═══════════════════════════════════════════════════════════════════════
	     REFERENCES TAB
	═══════════════════════════════════════════════════════════════════════ -->
	{#if activeTab === 'references'}
		<div class="flex-1 overflow-y-auto p-6">
			<div class="mx-auto max-w-3xl">
				<h2 class="mb-6 text-2xl font-bold">DSPy References</h2>
				<p class="mb-6 text-muted-foreground">
					Helpful resources for learning about DSPy and building effective prompts.
				</p>

				<div class="space-y-4">
					<div class="rounded-xl border bg-card p-5 shadow-sm transition-shadow hover:shadow-md">
						<div class="mb-2 flex items-center gap-2">
							<span class="flex h-6 w-6 items-center justify-center rounded-full bg-blue-100 text-xs font-bold text-blue-700">1</span>
							<h3 class="text-lg font-semibold">GitHub: PyDataFlowNote - DSPy</h3>
						</div>
						<p class="mb-3 text-sm text-muted-foreground">
							A collection of DSPy examples and patterns for building data flow notebooks with DSPy.
						</p>
						<a
							href="https://github.com/VidyasagarMSC/PyDataFlowNote/tree/main/dspy"
							target="_blank"
							rel="noopener noreferrer"
							class="inline-flex items-center gap-1.5 text-sm font-medium text-blue-600 hover:underline"
						>
							https://github.com/VidyasagarMSC/PyDataFlowNote/tree/main/dspy
							<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
							</svg>
						</a>
					</div>

					<div class="rounded-xl border bg-card p-5 shadow-sm transition-shadow hover:shadow-md">
						<div class="mb-2 flex items-center gap-2">
							<span class="flex h-6 w-6 items-center justify-center rounded-full bg-emerald-100 text-xs font-bold text-emerald-700">2</span>
							<h3 class="text-lg font-semibold">DZone: DSPy Framework Technical Guide</h3>
						</div>
						<p class="mb-3 text-sm text-muted-foreground">
							A comprehensive technical guide covering the DSPy framework, its core concepts, and best practices.
						</p>
						<a
							href="https://dzone.com/articles/dspy-framework-technical-guide"
							target="_blank"
							rel="noopener noreferrer"
							class="inline-flex items-center gap-1.5 text-sm font-medium text-blue-600 hover:underline"
						>
							https://dzone.com/articles/dspy-framework-technical-guide
							<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
							</svg>
						</a>
					</div>
				</div>
			</div>
		</div>
	{/if}
</div>

<!-- ─── View Modal ─────────────────────────────────────────────────────────── -->
{#if showViewModal && viewingPrompt}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
		role="dialog"
		aria-modal="true"
		aria-label="View Prompt"
	>
		<div class="w-full max-w-2xl rounded-2xl bg-background shadow-2xl mx-4 max-h-[85vh] flex flex-col">
			<div class="flex items-center justify-between border-b px-6 py-4">
				<h3 class="text-lg font-semibold">{viewingPrompt.prompt_name}</h3>
				<button
					class="rounded-md p-1 text-muted-foreground hover:bg-muted transition-colors cursor-pointer"
					onclick={() => (showViewModal = false)}>✕</button
				>
			</div>
			<div class="overflow-y-auto p-6 space-y-4 text-sm">
				<div class="grid grid-cols-2 gap-3">
					<div><span class="text-muted-foreground">Task Type:</span> <span class="font-medium ml-1">{viewingPrompt.task_type}</span></div>
					<div><span class="text-muted-foreground">Status:</span> <span class="font-medium ml-1">{viewingPrompt.status}</span></div>
					<div><span class="text-muted-foreground">Module:</span> <span class="font-mono font-medium ml-1 text-indigo-600">{viewingPrompt.module_type}</span></div>
					<div><span class="text-muted-foreground">Optimizer:</span> <span class="font-mono font-medium ml-1 text-violet-600">{viewingPrompt.optimizer}</span></div>
				</div>
				{#if viewingPrompt.prompt_desc}
					<div><div class="mb-1 text-muted-foreground">Description</div><p>{viewingPrompt.prompt_desc}</p></div>
				{/if}
				{#if viewingPrompt.signature_docstring}
					<div><div class="mb-1 text-muted-foreground">Docstring</div><p class="italic">{viewingPrompt.signature_docstring}</p></div>
				{/if}
				{#if viewingPrompt.optimized_instructions}
					<div>
						<div class="mb-1 text-muted-foreground font-medium">Optimized Instructions</div>
						<pre class="whitespace-pre-wrap rounded-lg bg-muted p-3 text-xs font-mono">{viewingPrompt.optimized_instructions}</pre>
					</div>
				{/if}
			</div>
			<div class="border-t px-6 py-4">
				<button
					class="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 cursor-pointer"
					onclick={() => (showViewModal = false)}>Close</button
				>
			</div>
		</div>
	</div>
{/if}

<!-- ─── Edit Modal ─────────────────────────────────────────────────────────── -->
{#if showEditModal && editingPrompt}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
		role="dialog"
		aria-modal="true"
		aria-label="Edit Prompt"
	>
		<div class="w-full max-w-xl rounded-2xl bg-background shadow-2xl mx-4">
			<div class="flex items-center justify-between border-b px-6 py-4">
				<h3 class="text-lg font-semibold">Edit Prompt</h3>
				<button
					class="rounded-md p-1 text-muted-foreground hover:bg-muted transition-colors cursor-pointer"
					onclick={() => (showEditModal = false)}>✕</button
				>
			</div>
			<div class="p-6 space-y-4">
				<div>
					<label for="edit-prompt-name" class="mb-1.5 block text-sm font-medium">Name</label>
					<input
						id="edit-prompt-name"
						type="text"
						class="w-full rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary"
						bind:value={editingPrompt.prompt_name}
					/>
				</div>
				<div>
					<label for="edit-prompt-desc" class="mb-1.5 block text-sm font-medium">Description</label>
					<textarea
						id="edit-prompt-desc"
						rows="3"
						class="w-full rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary"
						bind:value={editingPrompt.prompt_desc}
					></textarea>
				</div>
				<div>
					<label for="edit-prompt-instructions" class="mb-1.5 block text-sm font-medium">Optimized Instructions</label>
					<textarea
						id="edit-prompt-instructions"
						rows="5"
						class="w-full rounded-lg border bg-background px-3 py-2 text-sm font-mono outline-none focus:ring-2 focus:ring-primary"
						bind:value={editingPrompt.optimized_instructions}
					></textarea>
				</div>
			</div>
			<div class="flex gap-3 border-t px-6 py-4">
				<button
					class="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 cursor-pointer"
					onclick={updatePrompt}>Save Changes</button
				>
				<button
					class="rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-muted cursor-pointer"
					onclick={() => (showEditModal = false)}>Cancel</button
				>
			</div>
		</div>
	</div>
{/if}

<!-- ─── Delete Confirm ─────────────────────────────────────────────────────── -->
{#if showDeleteConfirm && deletingPromptId}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
		role="dialog"
		aria-modal="true"
		aria-label="Confirm Delete"
	>
		<div class="w-full max-w-sm rounded-2xl bg-background p-6 shadow-2xl mx-4">
			<h3 class="mb-2 text-lg font-semibold">Delete Prompt?</h3>
			<p class="mb-6 text-sm text-muted-foreground">
				This action cannot be undone. The prompt will be permanently removed.
			</p>
			<div class="flex gap-3">
				<button
					class="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 cursor-pointer"
					onclick={() => deletePrompt(deletingPromptId!)}>Delete</button
				>
				<button
					class="rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-muted cursor-pointer"
					onclick={() => { showDeleteConfirm = false; deletingPromptId = null; }}
				>Cancel</button>
			</div>
		</div>
	</div>
{/if}

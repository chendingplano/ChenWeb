<script lang="ts">
	let { darkMode = true }: { darkMode?: boolean } = $props();

	const pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	const cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	const textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	const textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
</script>

<section
	class="min-h-full w-full p-6 md:p-8"
	style="background:{pageBg}; color:{textPrimary};"
>
	<div
		class="mx-auto flex max-w-5xl flex-col gap-6 rounded-3xl p-6 md:p-8"
		style="background:{cardBg}; border:1px solid {borderColor};"
	>
		<div class="flex flex-col gap-3">
			<div
				class="inline-flex w-fit items-center rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em]"
				style="background:{accent}20; color:{accent};"
			>
				System Admin
			</div>
			<h1 class="text-2xl font-semibold md:text-3xl">Users, Roles and Access Controls</h1>
			<p class="max-w-3xl text-sm md:text-base" style="color:{textSecondary};">
				This workspace will host user administration, role assignment, and access-control
				management for Kratos-backed identities and shared authorization policies.
			</p>
		</div>

		<div class="grid gap-4 md:grid-cols-3">
			<div class="rounded-2xl p-4" style="border:1px solid {borderColor};">
				<h2 class="text-sm font-semibold uppercase tracking-[0.12em]" style="color:{accent};">
					Users
				</h2>
				<p class="mt-2 text-sm" style="color:{textSecondary};">
					Browse identities, review profile state, and manage compatibility flags such as
					<code>admin</code> and <code>is_owner</code>.
				</p>
			</div>
			<div class="rounded-2xl p-4" style="border:1px solid {borderColor};">
				<h2 class="text-sm font-semibold uppercase tracking-[0.12em]" style="color:{accent};">
					Roles
				</h2>
				<p class="mt-2 text-sm" style="color:{textSecondary};">
					Assign and audit role membership stored in Kratos identity metadata, including
					roles such as <code>admin</code>, <code>dev</code>, and <code>k_engineer</code>.
				</p>
			</div>
			<div class="rounded-2xl p-4" style="border:1px solid {borderColor};">
				<h2 class="text-sm font-semibold uppercase tracking-[0.12em]" style="color:{accent};">
					Access Controls
				</h2>
				<p class="mt-2 text-sm" style="color:{textSecondary};">
					Inspect and manage page-level access policies that map shared user roles onto
					configurable ChenWeb content.
				</p>
			</div>
		</div>

		<div class="rounded-2xl p-5" style="border:1px dashed {borderColor};">
			<p class="text-sm" style="color:{textSecondary};">
				Implementation placeholder: the navigation path is now wired and ready for the
				actual user-management UI.
			</p>
		</div>
	</div>
</section>

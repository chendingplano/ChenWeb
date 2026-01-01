<!----------------------------------------------------------
Source: https://superforms.rocks/get-started
File:   src/routes/test02/TestSuperForm.svelte 
Descript: A simple example of using sveltekit-superforms
Created: 2025/12/06 by Chen Ding
------------------------------------------------------------>
<script lang="ts" module>
	import Type from 'typebox';

	const schema = Type.Object({
  		name: Type.String({ default: 'Hello world!' }),
  		email: Type.String({ format: 'email' })
	});
	export { schema };
</script>

<script lang="ts">
	import { superForm } from 'sveltekit-superforms/client';

	let { data } = $props();

	const { form, errors, constraints, message, enhance } = superForm(data.form);
</script>

{#if $message}<h3>{$message}</h3>{/if}

<form method="POST" use:enhance>
	<label for="name">Name</label>
	<input
		type="text"
		name="name"
		aria-invalid={$errors.name ? 'true' : undefined}
		bind:value={$form.name}
		{...$constraints.name}
	/>
	{#if $errors.name}<span class="invalid">{$errors.name}</span>{/if}

	<label for="email">E-mail</label>
	<input
		type="email"
		name="email"
		aria-invalid={$errors.email ? 'true' : undefined}
		bind:value={$form.email}
		{...$constraints.email}
	/>
	{#if $errors.email}<span class="invalid">{$errors.email}</span>{/if}

	<div><button>Submit</button></div>
</form>

<hr />
<p>
	💥 <a target="_blank" href="https://superforms.rocks">Created with Superforms for SvelteKit</a> 💥
</p>

<style>
	.invalid {
		color: red;
	}
</style>

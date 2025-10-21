<script>
    import GoogleRegister from './GoogleRegister.svelte';
    import GitHubRegister from './GitHubRegister.svelte';
    import EmailRegister from './EmailRegister.svelte';

    let selectedMethod = 'email'; // Default to email registration
</script>

<div class="registration-container">
    <h1>Register</h1>
    <div class="registration-methods">
        <button on:click={() => selectedMethod = 'google'}>Register with Google</button>
        <button on:click={() => selectedMethod = 'github'}>Register with GitHub</button>
        <button on:click={() => selectedMethod = 'email'}>Register with Email</button>
    </div>

    {#if selectedMethod === 'google'}
        <GoogleRegister />
    {:else if selectedMethod === 'github'}
        <GitHubRegister />
    {:else}
        <EmailRegister />
    {/if}
</div>

<style>
    .registration-container {
        max-width: 400px;
        margin: 20px auto;
        padding: 20px;
        border: 1px solid #ddd;
        border-radius: 8px;
        box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
        text-align: center;
    }

    h1 {
        margin-bottom: 20px;
    }

    .registration-methods {
        margin-bottom: 20px;
    }

    button {
        margin: 5px;
        padding: 10px 15px;
        border: none;
        border-radius: 4px;
        background-color: #4CAF50;
        color: white;
        cursor: pointer;
        transition: background-color 0.3s;
    }

    button:hover {
        background-color: #45a049;
    }
</style>
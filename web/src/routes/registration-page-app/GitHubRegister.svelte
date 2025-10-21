<script>
    let isLoading = false;
    let errorMessage = '';

    async function handleGitHubRegister() {
        isLoading = true;
        errorMessage = '';
        try {
            const response = await fetch('/api/auth/github', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' }
            });
            if (!response.ok) throw new Error('GitHub registration failed. Please try again.');
            // Handle success (e.g., redirect or show message)
        } catch (error) {
            errorMessage = error.message;
        } finally {
            isLoading = false;
        }
    }
</script>

<div class="github-register">
    <h2>Register with GitHub</h2>
    <button on:click={handleGitHubRegister} disabled={isLoading}>
        {isLoading ? 'Registering...' : 'Register with GitHub'}
    </button>
    {#if errorMessage}
        <div class="error-message">{errorMessage}</div>
    {/if}
</div>

<style>
    .github-register { text-align: center; margin: 20px 0; }
    .error-message { color: red; margin-top: 10px; }
    button { padding: 10px 20px; font-size: 16px; cursor: pointer; }
</style>
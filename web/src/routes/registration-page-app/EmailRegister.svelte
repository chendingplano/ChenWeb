<script>
    let email = '';
    let password = '';
    let isLoading = false;
    let errorMessage = '';
    let successMessage = '';

    async function handleEmailRegister() {
        isLoading = true;
        errorMessage = '';
        successMessage = '';
        try {
            const response = await fetch('/api/auth/email', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email, password })
            });
            if (!response.ok) throw new Error('Email registration failed. Please try again.');
            successMessage = 'Registration successful!';
            email = '';
            password = '';
        } catch (error) {
            errorMessage = error.message;
        } finally {
            isLoading = false;
        }
    }
</script>

<div class="email-register">
    <h2>Register with Email</h2>
    <form on:submit|preventDefault={handleEmailRegister}>
        <input type="email" placeholder="Email" bind:value={email} required />
        <input type="password" placeholder="Password" bind:value={password} required />
        <button type="submit" disabled={isLoading}>
            {isLoading ? 'Registering...' : 'Register'}
        </button>
    </form>
    {#if errorMessage}
        <div class="error-message">{errorMessage}</div>
    {/if}
    {#if successMessage}
        <div class="success-message">{successMessage}</div>
    {/if}
</div>

<style>
    .email-register { text-align: center; margin: 20px 0; }
    input { margin: 8px 0; padding: 8px; width: 90%; }
    .error-message { color: red; margin-top: 10px; }
    .success-message { color: green; margin-top: 10px; }
    button { padding: 10px 20px; font-size: 16px; cursor: pointer; }
</style>
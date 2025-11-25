<script>
	import { goto } from "$app/navigation";

    let isLoading = false;
    let errorMessage = '';

    function handleGoogleRegister() {
        isLoading = true;
        errorMessage = '';
        // Redirect the user to the backend's Google OAuth endpoint
        // window.location.href = '/api/auth/google';
        goto('/api/auth/google');
    }
</script>

<div class="google-register">
    <h2>Register with Google AA</h2>
    <button on:click={handleGoogleRegister} disabled={isLoading}>
        {isLoading ? 'Redirecting...' : 'Register with Google'}
    </button>
    {#if errorMessage}
        <div class="error-message">{errorMessage}</div>
    {/if}
</div>

<style>
    .google-register {
        text-align: center;
        margin: 20px 0;
    }

    .error-message {
        color: red;
        margin-top: 10px;
    }

    button {
        padding: 10px 20px;
        font-size: 16px;
        cursor: pointer;
    }
</style>
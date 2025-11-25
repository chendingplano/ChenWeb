<script lang="ts">
  import { page } from '$app/stores';

  let password = '';
  let confirmPassword = '';
  let message = '';
  let isLoading = false;
  let showPassword = false;

  // Get the token from the query string (?token=...)
  $: token = $page.url.searchParams.get('token') || '';

  async function handleResetPassword() {
    message = '';

    if (!password) {
      message = 'Please enter a new password.';
      return;
    }

    if (password.length < 8) {
      message = 'Password must be at least 8 characters long.';
      return;
    }

    if (password !== confirmPassword) {
      message = 'Passwords do not match.';
      return;
    }

    isLoading = true;
    try {
      const res = await fetch('http://localhost:8080/auth/email/reset/confirm', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token, password })
      });

      if (res.ok) {
        message = 'Your password has been reset successfully! Redirecting to login...';
        setTimeout(() => {
          window.location.href = '/login';
        }, 2000);
      } else {
        const errorMsg = await res.text();
        message = errorMsg || 'Failed to reset password. Please try again.';
      }
    } catch (err) {
      message = 'Network error. Please check your connection and try again.';
    } finally {
      isLoading = false;
    }
  }
</script>

<div class="min-h-screen flex items-center justify-center bg-gradient-to-br from-gray-50 to-gray-100 p-4">
  <div class="w-full max-w-md bg-white rounded-2xl shadow-lg overflow-hidden border border-gray-200">
    <div class="bg-indigo-600 py-6 px-6 text-center">
      <h1 class="text-2xl font-bold text-white">Reset Your Password</h1>
      <p class="text-indigo-200 mt-1">Enter a new secure password below</p>
    </div>

    <div class="p-6">
      {#if message}
        <div
          class="mb-5 p-3 rounded-lg text-sm font-medium {
            message.includes('successfully')
              ? 'bg-green-100 text-green-800'
              : 'bg-red-100 text-red-800'
          }"
        >
          {message}
        </div>
      {/if}

      <form class="space-y-5" on:submit|preventDefault={handleResetPassword}>
        <div>
          <label for="password" class="block text-sm font-medium text-gray-700 mb-1">
            New Password
          </label>
          <div class="relative">
            <input
              id="password"
              type={showPassword ? 'text' : 'password'}
              placeholder="••••••••"
              bind:value={password}
              required
              class="w-full px-4 py-3 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition"
            />
            <button
              type="button"
              class="absolute right-3 top-3.5 text-gray-500 hover:text-gray-700"
              on:click={() => (showPassword = !showPassword)}
            >
              {#if showPassword}
                <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                </svg>
              {:else}
                <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21" />
                </svg>
              {/if}
            </button>
          </div>
          <p class="mt-1 text-xs text-gray-500">Must be at least 8 characters</p>
        </div>

        <div>
          <label for="confirmPassword" class="block text-sm font-medium text-gray-700 mb-1">
            Confirm New Password
          </label>
          <input
            id="confirmPassword"
            type="password"
            placeholder="••••••••"
            bind:value={confirmPassword}
            required
            class="w-full px-4 py-3 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition"
          />
        </div>

        <button
          type="submit"
          disabled={isLoading}
          class="w-full py-3 px-4 bg-indigo-600 hover:bg-indigo-700 text-white font-semibold rounded-lg shadow-md transition duration-200 disabled:opacity-70 disabled:cursor-not-allowed flex items-center justify-center"
        >
          {#if isLoading}
            <svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            Resetting...
          {:else}
            Reset Password
          {/if}
        </button>
      </form>

      <div class="mt-6 pt-5 border-t border-gray-200 text-center">
        <a href="/login" class="text-sm font-medium text-indigo-600 hover:text-indigo-500 transition">
          ← Back to Login
        </a>
      </div>
    </div>
  </div>
</div>
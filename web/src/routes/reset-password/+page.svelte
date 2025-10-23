<script lang="ts">
  import { page } from '$app/stores';

  let password = "";
  let confirmPassword = "";
  let message = "test";

  // Get the token from the query string (?token=...)
  $: token = $page.url.searchParams.get("token") || "";

  async function handleResetPassword() {
    if (password !== confirmPassword) {
      message = "Passwords do not match.";
      return;
    }

    const res = await fetch("http://localhost:8080/auth/email/reset/confirm", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token, password })
    });

    if (res.ok) {
      message = "Your password has been reset successfully. You can now log in.";
      // optional redirect after a delay
      setTimeout(() => {
        window.location.href = "/login";
      }, 2000);
    } else {
      message = await res.text();
    }
  }
</script>

<div class="form-container">
  <h2 class="title">Reset Your Password</h2>

  {#if message}
    <p class="message">{message}</p>
  {/if}

  <form class="form" on:submit|preventDefault={handleResetPassword}>
    <input
      type="password"
      placeholder="New Password"
      bind:value={password}
      required
      class="input"
    />
    <input
      type="password"
      placeholder="Confirm New Password"
      bind:value={confirmPassword}
      required
      class="input"
    />
    <button type="submit" class="form-btn">Reset Password</button>
  </form>
</div>

<style>
  .form-container { max-width: 400px; margin: auto; padding: 2rem; }
  .form-btn { width: 100%; margin-top: 1rem; }
  .message { color: green; margin-top: 1rem; }
</style>

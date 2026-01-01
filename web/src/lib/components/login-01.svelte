<script lang="ts">
	import { redirect } from "@sveltejs/kit";
	import { fromTheme } from "tailwind-merge";
  import { appAuthStore } from '@chendingplano/shared';
	import { pass } from "three/tsl";

  // This page implements three modes: login, signup, forgot password
  // (or three pages). This is controlled by the "mode" variable.

  let user_name = $state("");
  let first_name = $state("");
  let last_name = $state("");
  let email = $state("");
  let password = $state("");
  let mode = $state("login")

  async function handleEmailLogin() {
    try {
      const res = await fetch("http://localhost:5173/auth/email/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password })
      });

      if (res.ok) {
        const data = await res.json();
        let redirect_url = data.redirect_url || '/sidebar-01';
        redirect_url += `?name=${encodeURIComponent(data.name)}`;
        // alert("Login successful!");
        // window.location.href = `/sidebar-01?name=${encodeURIComponent(data.name)}`;
        window.location.href = redirect_url;
      } else {
        const msg = await res.text();
        alert(`Login failed: ${msg}`);
      }
    } catch (err) {
      alert(`Network error: ${err}`);
    }
  }

  async function handleEmailSignup() {
    /*
    const res = await fetch("http://localhost:8080/auth/email/signup", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, email, password })
    });

    const data = await res.json();
    if (res.ok) {
      if (data.message?.includes("user already exists")) {
        alert(data.message); // ← shows "user already exists (MID_EML_224)"
      } else {
        const msg = "An email has been sent to your email:" + email +
            ". Please check your email and click the link to activate your account." +
            "Note: if you cannot find the email, check the Junk Mail section! (MID_LGN_035)"
        alert(msg)
        mode = "login";
      }
    } else {
      alert(await res.text());
    }
  }
    */

    // await authStore.register(name, email, password);
    if (user_name) {
      console.log("User Name:", user_name);
      alert(`Sign up with user name: ${user_name} is currently disabled for demo purposes.`);
    } else if (email) {
      console.log("Email:", email);
      const passwordConfirm = password;
      await appAuthStore.register({email, password, passwordConfirm, first_name, last_name});
    } else {
      alert("Please provide a user name or email to sign up.");
    }
  }

  async function handleForgotPassword() {
    const res = await fetch("http://localhost:8080/auth/email/forgot", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email })
    });
    if (res.ok) {
      alert("A password reset link has been sent to your email.");
      mode = "login";
    } else {
      alert(await res.text());
    }
  }

  function switchToLogin() {
    mode = "login";
  }

  function switchToSignup() {
    mode = "signup";
  }

  function switchToForgot() {
    mode = "forgot";
  }
</script>

<div class="form-container">
    <p class="title">
      {mode === "login" ? "Welcome to DeepDocs" : "Create an Account"}
    </p>

    {#if mode === "login"}
      <!-- Login Form -->
      <form class="form" onsubmit={handleEmailLogin}>
        <input bind:value={email} type="email" class="input" placeholder="Email" required />
        <input bind:value={password} type="password" class="input" placeholder="Password" required />
        <p class="page-link">
          <button type="button" class="page-link-label" onclick={switchToForgot}>Forgot Password?</button>
        </p>
        <button class="form-btn">Log in</button>
      </form>

      <p class="sign-up-label">
        Don't have an account?
        <button class="sign-up-link" onclick={switchToSignup}>Sign up</button>
      </p>
    {:else if mode === "signup"}
      <!-- Sign Up Form -->
      <form class="form" onsubmit={handleEmailSignup}>
        <input bind:value={user_name} type="text" class="input" placeholder="user name" />
        <input bind:value={first_name} type="text" class="input" placeholder="First name" />
        <input bind:value={last_name} type="text" class="input" placeholder="Last name" />
        <input bind:value={email} type="email" class="input" placeholder="Email" />
        <input bind:value={password} type="password" class="input" placeholder="Password" required />
        <button class="form-btn">Sign up</button>
      </form>

      <p class="sign-up-label">
        Already have an account?
        <!--span class="sign-up-link" onclick={switchToLogin}>Log in</span-->
        <button class="sign-up-link" onclick={switchToLogin}>Log in</button>
      </p>
    {:else if mode == "forgot"}
      <!-- Forgot Password -->
      <form class="form" onsubmit={handleForgotPassword}>
        <input bind:value={email} type="email" class="input" placeholder="Enter your email" required />
        <button class="form-btn">Send Reset Link</button>
      </form>

      <p class="sign-up-label">
        Remember your password?
        <button class="sign-up-link" onclick={switchToLogin}>Log in</button>
      </p>
    {/if}

    <div class="buttons-container">
        <button
          type="button"
          class="github-login-button"
          onclick={() => { window.location.href = 'http://localhost:8080/auth/github/login'; }}>
          <svg stroke="currentColor" fill="currentColor" stroke-width="0" class="apple-icon" viewBox="0 0 1024 1024" height="1em" width="1em" xmlns="http://www.w3.org/2000/svg">
            <!--path d="M747.4 535.7c-.4-68.2 30.5-119.6 92.9-157.5-34.9-50-87.7-77.5-157.3-82.8-65.9-5.2-138 38.4-164.4 38.4-27.9 0-91.7-36.6-141.9-36.6C273.1 298.8 163 379.8 163 544.6c0 48.7 8.9 99 26.7 150.8 23.8 68.2 109.6 235.3 199.1 232.6 46.8-1.1 79.9-33.2 140.8-33.2 59.1 0 89.7 33.2 141.9 33.2 90.3-1.3 167.9-153.2 190.5-221.6-121.1-57.1-114.6-167.2-114.6-170.7zm-105.1-305c50.7-60.2 46.1-115 44.6-134.7-44.8 2.6-96.6 30.5-126.1 64.8-32.5 36.8-51.6 82.3-47.5 133.6 48.4 3.7 92.6-21.2 129-63.7z"></path> -->
            <path d="M511.6 76.3c-240.8 0-436.2 195.4-436.2 436.2 0 192.8 125.2 356.6 298.7 414.4 21.8 4 29.8-9.5 29.8-21.1 0-10.5-.4-45.3-.6-82.1-121.5 26.4-147.2-51.4-147.2-51.4-19.8-50.3-48.4-63.7-48.4-63.7-39.6-27.1 3-26.6 3-26.6 43.8 3.1 66.8 45 66.8 45 38.9 66.7 102 47.5 126.8 36.3 4-28.2 15.2-47.5 27.6-58.4-97-11-199-48.5-199-216 0-47.7 17-86.7 44.9-117.3-4.5-11-19.5-55.5 4.2-115.7 0 0 36.7-11.8 120.2 44.8 34.9-9.7 72.3-14.6 109.5-14.8 37.1.2 74.6 5.1 109.5 14.8 83.4-56.6 120.1-44.8 120.1-44.8 23.8 60.2 8.8 104.7 4.3 115.7 27.9 30.6 44.8 69.6 44.8 117.3 0 167.9-102.1 204.9-199.3 215.7 15.6 13.5 29.5 40.1 29.5 81.1 0 58.6-.5 105.9-.5 120.4 0 11.7 7.9 25.3 30 21 173.4-57.8 298.6-221.6 298.6-414.3 0-240.8-195.4-436.2-436.2-436.2z"></path>
          </svg>
          <span>Log in with GitHub</span>
        </button>

        <button
          type="button"
          class="google-login-button"
          onclick={() => { window.location.href = 'http://localhost:8080/auth/google/login'; }}
        >
          <svg stroke="currentColor" fill="currentColor" stroke-width="0" version="1.1" x="0px" y="0px" class="google-icon" viewBox="0 0 48 48" height="1em" width="1em" xmlns="http://www.w3.org/2000/svg">
            <path fill="#FFC107" d="M43.611,20.083H42V20H24v8h11.303c-1.649,4.657-6.08,8-11.303,8c-6.627,0-12-5.373-12-12
	c0-6.627,5.373-12,12-12c3.059,0,5.842,1.154,7.961,3.039l5.657-5.657C34.046,6.053,29.268,4,24,4C12.955,4,4,12.955,4,24
	c0,11.045,8.955,20,20,20c11.045,0,20-8.955,20-20C44,22.659,43.862,21.35,43.611,20.083z"></path>
            <path fill="#FF3D00" d="M6.306,14.691l6.571,4.819C14.655,15.108,18.961,12,24,12c3.059,0,5.842,1.154,7.961,3.039l5.657-5.657
	C34.046,6.053,29.268,4,24,4C16.318,4,9.656,8.337,6.306,14.691z"></path>
            <path fill="#4CAF50" d="M24,44c5.166,0,9.86-1.977,13.409-5.192l-6.19-5.238C29.211,35.091,26.715,36,24,36
	c-5.202,0-9.619-3.317-11.283-7.946l-6.522,5.025C9.505,39.556,16.227,44,24,44z"></path>
            <path fill="#1976D2" d="M43.611,20.083H42V20H24v8h11.303c-0.792,2.237-2.231,4.166-4.087,5.571
	c0.001-0.001,0.002-0.001,0.003-0.002l6.19,5.238C36.971,39.205,44,34,44,24C44,22.659,43.862,21.35,43.611,20.083z"></path>
          </svg>
          <span>Log in with Google AAA</span>
        </button>
      </div>
</div>
<style>
  .form-container {
    width: 350px;
    background-color: #fff;
    box-shadow: rgba(0, 0, 0, 0.35) 0px 5px 15px;
    border-radius: 10px;
    box-sizing: border-box;
    padding: 20px 30px;
  }

  .title {
    text-align: center;
    font-family: "Lucida Sans", "Lucida Sans Regular", "Lucida Grande",
          "Lucida Sans Unicode", Geneva, Verdana, sans-serif;
    margin: 10px 0 30px 0;
    font-size: 20px;
    font-weight: 800;
  }

  .form {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 18px;
    margin-bottom: 15px;
  }

  .input {
    border-radius: 20px;
    border: 1px solid #c0c0c0;
    outline: 0 !important;
    box-sizing: border-box;
    padding: 12px 15px;
  }

  .page-link {
    text-decoration: underline;
    margin: 0;
    text-align: end;
    color: #747474;
    text-decoration-color: #747474;
  }

  .page-link-label {
    cursor: pointer;
    font-family: "Lucida Sans", "Lucida Sans Regular", "Lucida Grande",
          "Lucida Sans Unicode", Geneva, Verdana, sans-serif;
    font-size: 9px;
    font-weight: 700;
  }

  .page-link-label:hover {
    color: #000;
  }

  .form-btn {
    padding: 10px 15px;
    font-family: "Lucida Sans", "Lucida Sans Regular", "Lucida Grande",
          "Lucida Sans Unicode", Geneva, Verdana, sans-serif;
    border-radius: 20px;
    border: 0 !important;
    outline: 0 !important;
    background: teal;
    color: white;
    cursor: pointer;
    box-shadow: rgba(0, 0, 0, 0.24) 0px 3px 8px;
  }

  .form-btn:active {
    box-shadow: none;
  }

  .sign-up-label {
    margin: 0;
    font-size: 10px;
    color: #747474;
    font-family: "Lucida Sans", "Lucida Sans Regular", "Lucida Grande",
          "Lucida Sans Unicode", Geneva, Verdana, sans-serif;
  }

  .sign-up-link {
    margin-left: 1px;
    font-size: 11px;
    text-decoration: underline;
    text-decoration-color: teal;
    color: teal;
    cursor: pointer;
    font-weight: 800;
    font-family: "Lucida Sans", "Lucida Sans Regular", "Lucida Grande",
          "Lucida Sans Unicode", Geneva, Verdana, sans-serif;
  }

  .buttons-container {
    width: 100%;
    display: flex;
    flex-direction: column;
    justify-content: flex-start;
    margin-top: 20px;
    gap: 15px;
  }

  .google-login-button {
    border-radius: 20px;
    box-sizing: border-box;
    padding: 10px 15px;
    box-shadow: rgba(0, 0, 0, 0.16) 0px 10px 36px 0px,
          rgba(0, 0, 0, 0.06) 0px 0px 0px 1px;
    cursor: pointer;
    display: flex;
    justify-content: center;
    align-items: center;
    font-family: "Lucida Sans", "Lucida Sans Regular", "Lucida Grande",
          "Lucida Sans Unicode", Geneva, Verdana, sans-serif;
    font-size: 11px;
    gap: 5px;
  }

  .google-login-button {
    border: 2px solid #747474;
  }

  .github-login-button {
  border-radius: 20px;
  box-sizing: border-box;
  padding: 10px 15px;
  box-shadow: rgba(0, 0, 0, 0.16) 0px 10px 36px 0px,
        rgba(0, 0, 0, 0.06) 0px 0px 0px 1px;
  cursor: pointer;
  display: flex;
  justify-content: center;
  align-items: center;
  font-family: "Lucida Sans", "Lucida Sans Regular", "Lucida Grande",
        "Lucida Sans Unicode", Geneva, Verdana, sans-serif;
  font-size: 11px;
  gap: 5px;
}

.github-login-button {
  background-color: #000;
  color: #fff;
  border: 2px solid #000;
}
</style>

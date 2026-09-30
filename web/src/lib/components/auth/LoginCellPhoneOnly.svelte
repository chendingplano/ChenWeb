<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	// The phone-only login screen (config value "login-cell-phone-only"):
	// Chinese mobile number + SMS code, nothing else. Sign-up is implicit —
	// the /auth/phone/send-code backend starts a registration flow when the
	// number is unknown, so an unrecognized number creates an account on first
	// verify. Reached via the LoginPanel selector, never imported directly.
	// Deliberately does NOT read [frontend].enable_phone_login: selecting this
	// screen in [system].login_page is itself the opt-in.
	//
	// Render for a full-page login (`variant="page"`, the default) or inside a
	// modal (`variant="modal"`), which shows its own "Cancel" button wired to
	// `onClose`. Keep the panel shell/styling in sync with LoginEmailGoogle so
	// the two screens look like one system.
	let {
		variant = 'page',
		onClose
	}: { variant?: 'page' | 'modal'; onClose?: () => void } = $props();

	// Phase 1: Chinese mobile numbers only, matching /auth/phone/* + Kratos.
	const cnPhonePattern = /^1[3-9]\d{9}$/;
	let phone = $state('');
	let phoneCode = $state('');
	// Collected only when phoneFlowType === 'registration' (new account) -
	// shown on the code-entry step, submitted together with the code.
	let firstName = $state('');
	let lastName = $state('');
	let phoneStep = $state<'enter-phone' | 'enter-code'>('enter-phone');
	let phoneFlowId = $state('');
	let phoneFlowType = $state('');
	let phoneSending = $state(false);
	let phoneCooldownSeconds = $state(0);
	let phoneCooldownTimer: ReturnType<typeof setInterval> | undefined;

	function startPhoneCooldown() {
		phoneCooldownSeconds = 60;
		if (phoneCooldownTimer) clearInterval(phoneCooldownTimer);
		phoneCooldownTimer = setInterval(() => {
			phoneCooldownSeconds -= 1;
			if (phoneCooldownSeconds <= 0 && phoneCooldownTimer) {
				clearInterval(phoneCooldownTimer);
				phoneCooldownTimer = undefined;
			}
		}, 1000);
	}

	async function handleSendPhoneCode() {
		if (!cnPhonePattern.test(phone)) {
			alert(m.logincellphoneonly_please_enter_a_valid_chinese());
			return;
		}
		phoneSending = true;
		try {
			const res = await fetch('/auth/phone/send-code', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ phone })
			});
			const data = await res.json();
			if (res.ok) {
				phoneFlowId = data.flow_id;
				phoneFlowType = data.flow_type;
				phoneStep = 'enter-code';
				startPhoneCooldown();
			} else {
				alert(data.message || m.logincellphoneonly_failed_to_send_code());
			}
		} catch (err) {
			alert(m.logincellphoneonly_network_error({ err: String(err) }));
		} finally {
			phoneSending = false;
		}
	}

	async function handleVerifyPhoneCode() {
		if (!phoneCode) {
			alert(m.logincellphoneonly_please_enter_the_code_you());
			return;
		}
		if (phoneFlowType === 'registration' && (!firstName || !lastName)) {
			alert(m.logincellphoneonly_please_enter_your_first_and());
			return;
		}
		try {
			const res = await fetch('/auth/phone/verify', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					phone,
					code: phoneCode,
					flow_id: phoneFlowId,
					flow_type: phoneFlowType,
					...(phoneFlowType === 'registration'
						? { first_name: firstName, last_name: lastName }
						: {})
				})
			});
			const data = await res.json();
			if (res.ok) {
				window.location.href = data.redirect_url || '/sidebar-01';
			} else {
				alert(data.message || m.logincellphoneonly_invalid_or_expired_code());
			}
		} catch (err) {
			alert(m.logincellphoneonly_network_error({ err: String(err) }));
		}
	}

	function useDifferentNumber() {
		phoneCode = '';
		phoneFlowId = '';
		phoneFlowType = '';
		firstName = '';
		lastName = '';
		phoneStep = 'enter-phone';
	}
</script>

<div class="form-container">
	<p class="title">{m.logincellphoneonly_welcome_to_deepdocs()}</p>

	{#if phoneStep === 'enter-phone'}
		<form
			class="form"
			onsubmit={(event) => {
				event.preventDefault();
				void handleSendPhoneCode();
			}}
		>
			<input
				bind:value={phone}
				type="tel"
				class="input"
				placeholder={m.logincellphoneonly_mobile_number_e_g_13812345678()}
				autocomplete="tel"
				inputmode="numeric"
				maxlength="11"
				required
			/>
			<button type="submit" class="form-btn" disabled={phoneSending}>
				{phoneSending ? m.logincellphoneonly_sending() : m.logincellphoneonly_send_code()}
			</button>
		</form>

		<p class="hint">{m.logincellphoneonly_new_number_signing_in_creates()}</p>
	{:else}
		<form
			class="form"
			onsubmit={(event) => {
				event.preventDefault();
				void handleVerifyPhoneCode();
			}}
		>
			<input
				bind:value={phoneCode}
				type="text"
				class="input"
				placeholder={m.logincellphoneonly_enter_the_code_you_received()}
				autocomplete="one-time-code"
				inputmode="numeric"
				required
			/>
			{#if phoneFlowType === 'registration'}
				<input
					bind:value={firstName}
					type="text"
					class="input"
					placeholder={m.logincellphoneonly_first_name()}
					autocomplete="given-name"
					required
				/>
				<input
					bind:value={lastName}
					type="text"
					class="input"
					placeholder={m.logincellphoneonly_last_name()}
					autocomplete="family-name"
					required
				/>
			{/if}
			<button type="submit" class="form-btn">{m.logincellphoneonly_verify_code()}</button>
			<button
				type="button"
				class="page-link-label"
				disabled={phoneCooldownSeconds > 0}
				onclick={() => {
					void handleSendPhoneCode();
				}}
			>
				{phoneCooldownSeconds > 0 ? m.logincellphoneonly_resend_code_s({ phoneCooldownSeconds }) : m.logincellphoneonly_resend_code()}
			</button>
		</form>

		<p class="sign-up-label">
			<button class="sign-up-link" onclick={useDifferentNumber}>{m.logincellphoneonly_use_a_different_number()}</button>
		</p>
	{/if}

	{#if variant === 'modal' && onClose}
		<button type="button" class="cancel-btn" onclick={onClose}>{m.logincellphoneonly_cancel()}</button>
	{/if}
</div>

<style>
	.form-container {
		width: min(350px, calc(100vw - 2rem));
		background-color: #fff;
		color: #1a1a1a;
		box-shadow: rgba(0, 0, 0, 0.35) 0px 5px 15px;
		border-radius: 10px;
		box-sizing: border-box;
		padding: 20px 30px;
	}

	.title {
		text-align: center;
		font-family:
			'Lucida Sans', 'Lucida Sans Regular', 'Lucida Grande', 'Lucida Sans Unicode', Geneva, Verdana,
			sans-serif;
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

	.page-link-label {
		cursor: pointer;
		font-family:
			'Lucida Sans', 'Lucida Sans Regular', 'Lucida Grande', 'Lucida Sans Unicode', Geneva, Verdana,
			sans-serif;
		font-size: 9px;
		font-weight: 700;
	}

	.page-link-label:hover {
		color: #000;
	}

	.form-btn {
		padding: 10px 15px;
		font-family:
			'Lucida Sans', 'Lucida Sans Regular', 'Lucida Grande', 'Lucida Sans Unicode', Geneva, Verdana,
			sans-serif;
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

	.hint {
		margin: 0;
		font-size: 10px;
		color: #747474;
		text-align: center;
		font-family:
			'Lucida Sans', 'Lucida Sans Regular', 'Lucida Grande', 'Lucida Sans Unicode', Geneva, Verdana,
			sans-serif;
	}

	.sign-up-label {
		margin: 0;
		font-size: 10px;
		color: #747474;
		font-family:
			'Lucida Sans', 'Lucida Sans Regular', 'Lucida Grande', 'Lucida Sans Unicode', Geneva, Verdana,
			sans-serif;
	}

	.sign-up-link {
		margin-left: 1px;
		font-size: 11px;
		text-decoration: underline;
		text-decoration-color: teal;
		color: teal;
		cursor: pointer;
		font-weight: 800;
		font-family:
			'Lucida Sans', 'Lucida Sans Regular', 'Lucida Grande', 'Lucida Sans Unicode', Geneva, Verdana,
			sans-serif;
	}

	/* Modal-only dismiss control. Mirrors .form-btn geometry so it lines up
	   with the rest of the panel; neutral colours so it reads as secondary. */
	.cancel-btn {
		width: 100%;
		margin-top: 15px;
		padding: 10px 15px;
		font-family:
			'Lucida Sans', 'Lucida Sans Regular', 'Lucida Grande', 'Lucida Sans Unicode', Geneva, Verdana,
			sans-serif;
		border-radius: 20px;
		border: 0 !important;
		outline: 0 !important;
		background: #e2e2e2;
		color: #4a4a4a;
		cursor: pointer;
		box-shadow: rgba(0, 0, 0, 0.24) 0px 3px 8px;
	}

	.cancel-btn:hover {
		background: #d5d5d5;
		color: #1a1a1a;
	}

	.cancel-btn:active {
		box-shadow: none;
	}
</style>

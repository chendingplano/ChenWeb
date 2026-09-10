# Enabling phone (SMS-code) login/sign-up on the China box

Target: `onto.bzton.cn` / `210.5.158.91` (hostname `rssvr19`), ChenWeb on port **8090**.

The phone-login **code is already in the binaries** (frontend embedded, backend
handlers + SMS relay route compiled in). This directory covers the four
box-side changes that a binary swap does **not** carry:

| Step | File | What | Restart |
|-----|------|------|---------|
| 1 | `01-chenweb-config.md` | `enable_phone_login = true` in the box `config.local.toml` | `chenweb` |
| 2 | `02-chenweb-env.md` | `SMS_RELAY_SHARED_SECRET` + `ALIYUN_SMS_*` in the box's chenweb env | `chenweb` |
| 3 | `03-kratos-config.md` + `request.config.jsonnet` | phone trait, `code` passwordless login, SMS courier channel → the relay | `kratos` |
| 4 | `verify-phone-login.sh` | on-box end-to-end smoke test | — |

## Order of operations

1. Deploy the new binaries (`deploy-server-china.sh`) — see the parent `scripts/`.
2. Apply steps 1–3 below.
3. `sudo systemctl restart kratos && sleep 3 && sudo systemctl restart chenweb`
4. Run `verify-phone-login.sh` on the box.
5. Test from a browser: `https://onto.bzton.cn/` → "Log in with Phone".

## Why this should work from this box when it failed from the Mac

The Aliyun key (`LTAI5tPs…dArT`, shared with bzton prod — see
`../../ ..` investigation notes) returns `InvalidAccessKeyId.AccessPolicyDenied`
for `SendSms` calls **from the Mac's US egress IP**. This box is
`210.5.158.91` — the same public IP as `www.bzton.com`, whose production
sends with that exact key every day. So the relay's `SendSms` call from here
originates from an already-trusted IP. If it still fails, `03` and
`verify-phone-login.sh` will show exactly where.

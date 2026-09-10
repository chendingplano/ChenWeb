# Step 2 — SMS relay env vars for the `chenweb` service

`shared/go/api/auth/sms_relay.go` reads these from the process environment
(fresh per request, not cached):

| var | value | notes |
|-----|-------|-------|
| `SMS_RELAY_SHARED_SECRET` | `<redacted — see secrets store>` | must **exactly** match the `X-Internal-Relay-Secret` header in the Kratos SMS courier channel (step 3). Rotate per-env if you prefer — just keep both sides equal. |
| `ALIYUN_SMS_ACCESS_KEY_ID` | `<redacted — see secrets store>` | same key bzton production uses (`bzton-be/.../AliyunSms/AliyunSmsEntity.java`) |
| `ALIYUN_SMS_ACCESS_KEY_SECRET` | `<redacted — see secrets store>` | |
| `ALIYUN_SMS_SIGN_NAME` | `润申标准化` | approved signature on that Aliyun account |
| `ALIYUN_SMS_TEMPLATE_CODE` | `SMS_223202121` | verification-code template; body uses `${code}` |

Missing any one → the relay logs `aliyun sms config missing` and returns 500,
Kratos abandons the message, no SMS.

## On the box

Find how `chenweb` gets its environment:

```bash
grep -E 'EnvironmentFile|Environment=' /etc/systemd/system/chenweb.service
```

**If it has `EnvironmentFile=<path>`** (commonly `~/Workspace/ChenWeb/.env`):
append the five lines to that file (no `export`, no surrounding quotes needed
for systemd):

```bash
F=~/Workspace/ChenWeb/.env          # <-- whatever EnvironmentFile points at
cp "$F" "$F.bak-$(date +%Y%m%d-%H%M%S)"
cat >> "$F" <<'EOF'

# phone/SMS login relay -> Aliyun Dysmsapi (added for phone-login enablement)
SMS_RELAY_SHARED_SECRET=<redacted — see secrets store>
ALIYUN_SMS_ACCESS_KEY_ID=<redacted — see secrets store>
ALIYUN_SMS_ACCESS_KEY_SECRET=<redacted — see secrets store>
ALIYUN_SMS_SIGN_NAME=润申标准化
ALIYUN_SMS_TEMPLATE_CODE=SMS_223202121
EOF
```

**If it uses `Environment=` lines / a drop-in** instead: add them via

```bash
sudo systemctl edit chenweb
# in the editor:
# [Service]
# Environment=SMS_RELAY_SHARED_SECRET=90cd...
# Environment=ALIYUN_SMS_ACCESS_KEY_ID=<redacted — see secrets store>
# ...etc
```

Then:

```bash
sudo systemctl daemon-reload
sudo systemctl restart chenweb
```

## Verify

```bash
# from the box; relay rejects a wrong secret with 401, right secret + this
# fake payload gets past auth to the (real) Aliyun call:
curl -s -o /dev/null -w '%{http_code}\n' -X POST \
  http://127.0.0.1:8090/auth/internal/sms-courier/send \
  -H 'X-Internal-Relay-Secret: wrong' -H 'Content-Type: application/json' \
  -d '{"to":"+8613800000000","code":"000000"}'
# want: 401  (proves SMS_RELAY_SHARED_SECRET is set and enforced)
```

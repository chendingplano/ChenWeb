#!/usr/bin/env bash
#
# On-box end-to-end check for phone (SMS-code) login/sign-up.
# Run ON THE China box after steps 1-3 + restarts.
#
#   bash verify-phone-login.sh [11-digit-CN-mobile]
#
# With no number: checks config/relay/Kratos wiring only (no SMS sent).
# With a real number: also triggers a send and reports whether Aliyun
# accepted it (courier_messages.status 1=queued 2=SENT 3=processing 4=ABANDONED).

set -uo pipefail

PORT=${PORT:-8090}
KRATOS_ADMIN=${KRATOS_ADMIN:-http://127.0.0.1:4434}
PHONE="${1:-}"
pass=0 fail=0
ok(){ echo "  ok   $*"; pass=$((pass+1)); }
bad(){ echo "  FAIL $*"; fail=$((fail+1)); }

echo "== 1. chenweb config =="
cfg=$(curl -s "http://127.0.0.1:$PORT/api/config" || true)
case "$cfg" in
  *'"enable_phone_login":true'*) ok "enable_phone_login = true" ;;
  *'"enable_phone_login"'*)      bad "enable_phone_login present but not true" ;;
  *)                             bad "/api/config unreachable or field missing" ;;
esac

echo "== 2. SMS relay env (wrong secret must 401) =="
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST \
  "http://127.0.0.1:$PORT/auth/internal/sms-courier/send" \
  -H 'X-Internal-Relay-Secret: deliberately-wrong' \
  -H 'Content-Type: application/json' \
  -d '{"to":"+8613800000000","code":"000000"}' || true)
case "$code" in
  401) ok "relay rejects bad secret (SMS_RELAY_SHARED_SECRET is set)" ;;
  500) bad "relay 500 -> SMS_RELAY_SHARED_SECRET not set in chenweb env (step 2)" ;;
  *)   bad "relay returned $code (expected 401)" ;;
esac

echo "== 3. Kratos 'code' method on the login flow =="
lf=$(curl -s "http://127.0.0.1:4433/self-service/login/api" || true)
case "$lf" in
  *'"group":"code"'*) ok "login flow offers the code method" ;;
  *) bad "login flow has no code method -> kratos.yml methods.code.passwordless_enabled (step 3b)" ;;
esac

echo "== 4. Kratos identity schema has the phone trait =="
sch=$(curl -s "$KRATOS_ADMIN/schemas/default" 2>/dev/null || \
      curl -s "$KRATOS_ADMIN/identity/schemas/default" 2>/dev/null || true)
case "$sch" in
  *'"phone"'*'"via":"sms"'*|*'"via": "sms"'*) ok "phone trait present (via sms)" ;;
  *'"phone"'*) ok "phone trait present" ;;
  *) bad "phone trait not found in default identity schema (step 3a)" ;;
esac

# ---- Kratos DB (for courier status) ----
DSN=$(tr '\0' '\n' < "/proc/$(pgrep -f 'kratos serve' | head -1)/environ" 2>/dev/null | sed -n 's/^DSN=//p')
[ -z "${DSN:-}" ] && DSN=$(grep -hoE 'postgres(ql)?://[^" ]+' \
  ~/Workspace/Kratos/mise.local.toml /etc/systemd/system/kratos.service 2>/dev/null | head -1)
psql_k(){ [ -n "${DSN:-}" ] && psql "$DSN" -Atqc "$1" 2>/dev/null; }

if [ -z "$PHONE" ]; then
  echo
  echo "== summary: $pass ok, $fail FAIL (no number given -> no send test) =="
  [ "$fail" -eq 0 ] && echo "wiring looks good; re-run with your CN mobile to test a real send."
  exit $([ "$fail" -eq 0 ] && echo 0 || echo 1)
fi

echo "== 5. trigger send for $PHONE =="
resp=$(curl -s -X POST "http://127.0.0.1:$PORT/auth/phone/send-code" \
  -H 'Content-Type: application/json' -d "{\"phone\":\"$PHONE\"}")
echo "  send-code response: $resp"
case "$resp" in
  *'"status":"ok"'*) ok "send-code accepted (flow created)" ;;
  *) bad "send-code did not return ok" ;;
esac

e164="+86$PHONE"
echo "== 6. Kratos courier result for $e164 (waiting up to 20s) =="
if [ -z "${DSN:-}" ]; then
  echo "  (could not locate Kratos DSN; check manually:"
  echo "     psql <kratos-dsn> -c \"select status,send_count,created_at from courier_messages where recipient='$e164' order by created_at desc limit 1\")"
else
  for _ in $(seq 1 10); do
    row=$(psql_k "select status||'|'||send_count from courier_messages where recipient='$e164' order by created_at desc limit 1")
    st=${row%%|*}
    case "$st" in
      2) ok "courier status=SENT (Aliyun accepted it) -- check the handset"; break ;;
      4) bad "courier status=ABANDONED -- send failed; dispatch error below"; break ;;
      1|3) echo "  ...status=$st (queued/processing), waiting"; sleep 2 ;;
      *) echo "  ...no row yet, waiting"; sleep 2 ;;
    esac
  done
  mid=$(psql_k "select id from courier_messages where recipient='$e164' order by created_at desc limit 1")
  [ -n "$mid" ] && psql_k "select status,error from courier_message_dispatches where message_id='$mid' order by created_at desc limit 2" \
    | sed 's/^/  dispatch: /'
fi

echo "== 7. recent chenweb SMS/phone log lines =="
sudo journalctl -u chenweb -n 200 --no-pager 2>/dev/null \
  | grep -E 'SHD_SMS_|SHD_PHN_|aliyun sms|sms code dispatched|sms send rate limit|no identity for phone' \
  | tail -15 | sed 's/^/  /'

echo
echo "== summary: $pass ok, $fail FAIL =="
exit $([ "$fail" -eq 0 ] && echo 0 || echo 1)

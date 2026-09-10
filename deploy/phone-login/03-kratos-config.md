# Step 3 — Kratos config on the box (phone trait + SMS code method + courier)

> **This is a spec of the required end state, not a blind patch.** The box's
> Kratos is *not* a copy of the Mac's (`HowTo.md`: "config in `mise.local.toml`
> … not a copy of the Mac's"; "Google OAuth and outbound email … left
> unconfigured — password login only"). Get the box's real files first:
>
> ```bash
> ssh -p 8822 gui@210.5.158.91 \
>   'cat ~/Workspace/Kratos/kratos/kratos.yml; echo "=====";
>    cat ~/Workspace/Kratos/kratos/identity.schema.json; echo "=====";
>    grep -iE "SELFSERVICE|COURIER|CODE|SCHEMA" ~/Workspace/Kratos/mise.local.toml'
> ```
>
> Then apply the four changes below as edits to those actual files.

All values below are the Mac's working configuration
(`/Users/cding/Workspace/Kratos/kratos/…`), verified end-to-end there in July.

---

## 3a. `identity.schema.json` — add the `phone` trait

Under `properties.traits.properties`, alongside `email` / `username`:

```json
"phone": {
  "type": "string",
  "title": "Phone Number",
  "description": "E.164-formatted Chinese mobile number (e.g. +8613812345678).",
  "pattern": "^\\+861[3-9][0-9]{9}$",
  "ory.sh/kratos": {
    "credentials": {
      "code": { "identifier": true, "via": "sms" }
    }
  }
}
```

And add `phone` as an accepted identifier in the `anyOf` at the end of
`traits`:

```json
"anyOf": [
  { "required": ["email"] },
  { "required": ["username"] },
  { "required": ["phone"] }
]
```

`identity.schema.json` hot-reloads — no restart needed for this file alone
(the yaml changes below do need one).

---

## 3b. `kratos.yml` — enable the `code` method for **passwordless login**

Under `selfservice.methods`:

```yaml
    code:
      enabled: true
      passwordless_enabled: true   # <-- required for LOGIN via code, not just
                                   #     registration/recovery/verification.
                                   #     Without it Strategy.Login() returns
                                   #     ErrStrategyNotResponsible and the
                                   #     login->registration fallback in
                                   #     kratos_phone.go breaks.
```

If Kratos config is being supplied by env var instead of yaml, the
equivalents are:

```
SELFSERVICE_METHODS_CODE_ENABLED=true
SELFSERVICE_METHODS_CODE_PASSWORDLESS_ENABLED=true
```

---

## 3c. `kratos.yml` — session hook after code-based registration & login

Under `selfservice.flows.registration.after` (and `…login.after` if present),
add a `code` branch next to `password` / `oidc`:

```yaml
      code:
        hooks:
          - hook: session
```

This is what makes a successful phone **sign-up** immediately return a
session (so the user lands logged in, not on a "now log in" page).

---

## 3d. `kratos.yml` — the SMS courier channel → the relay

Under `courier` (add a `channels:` list; keep any existing `smtp:` block):

```yaml
courier:
  # keep existing template_override_path / smtp if present
  channels:
    - id: sms
      type: http
      request_config:
        url: http://127.0.0.1:8090/auth/internal/sms-courier/send
        method: POST
        headers:
          X-Internal-Relay-Secret: <redacted — see secrets store>
        body: file://ABSOLUTE/PATH/TO/request.config.jsonnet
```

- `url` port is **8090** (not 8080 — this box moved ports on 2026-09-08).
- `X-Internal-Relay-Secret` must equal `SMS_RELAY_SHARED_SECRET` from step 2.
- Copy `request.config.jsonnet` (in this directory) onto the box, e.g. to
  `~/Workspace/Kratos/kratos/templates/courier/sms/request.config.jsonnet`,
  and put its **absolute** path in `body: file://…`.
- Courier channels are a nested list — setting this via env vars is brittle;
  put it in the yaml.

```bash
# on the box
mkdir -p ~/Workspace/Kratos/kratos/templates/courier/sms
# scp request.config.jsonnet there, then edit kratos.yml as above
```

---

## Apply

```bash
cd ~/Workspace/Kratos/kratos
cp kratos.yml kratos.yml.bak-$(date +%Y%m%d-%H%M%S)
cp identity.schema.json identity.schema.json.bak-$(date +%Y%m%d-%H%M%S)
# ...make the edits...
~/Workspace/Kratos/src/kratos/kratos validate --config ./kratos.yml   # if the subcommand exists
sudo systemctl restart kratos
sleep 3
sudo systemctl restart chenweb
sudo journalctl -u kratos -n 50 --no-pager
```

## Quick check

```bash
# login flow should now offer the "code" method
curl -s http://127.0.0.1:4433/self-service/login/api | \
  grep -o '"group":"code"' | head -1
# want: "group":"code"
```

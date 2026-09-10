# Step 1 — enable the phone-login UI + backend flag

ChenWeb reads `enable_phone_login` from `[frontend]` in `config.toml`, then
`config.local.toml` merged on top (`server/cmd/config/config.go` —
`ReadInConfig` + `MergeInConfig`). `config.local.toml` is box-local and is
never touched by a deploy, so edit it there.

## On the box

```bash
cd ~/Workspace/ChenWeb
cp config.local.toml config.local.toml.bak-$(date +%Y%m%d-%H%M%S)
```

Ensure the `[frontend]` table contains:

```toml
[frontend]
# ...whatever is already here...
enable_phone_login = true
```

If there is no `[frontend]` table in `config.local.toml`, append one:

```bash
printf '\n[frontend]\nenable_phone_login = true\n' >> config.local.toml
```

## Verify (after `sudo systemctl restart chenweb`)

```bash
curl -s http://127.0.0.1:8090/api/config | grep -o '"enable_phone_login":[^,}]*'
# want: "enable_phone_login":true
```

The login page's "Log in with Phone" link is gated on this field
(`web/src/lib/components/login-01.svelte`), so it stays hidden until the
flag is true.

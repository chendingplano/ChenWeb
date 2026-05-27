# Go / VS Code Debug Troubleshooting

This document records bugs that broke VS Code debugging in this project and explains how to fix them if they recur. The bugs stem from Go version conflicts made worse by the mix of Nix, Nix flakes, direnv, and mise managing Go installations.

---

## Environment overview

This project uses **three** Go version managers simultaneously:

| Manager | Config | Primary role |
|---|---|---|
| **home-manager (Nix)** | `~/Workspace/nix/` flake | System-level Go in `/etc/profiles/per-user/cding/bin/go` |
| **mise** | `ChenWeb/mise.toml`, subdirectory `mise.toml` files | Per-directory Go pins |
| **Nix flake (ChenWeb)** | `ChenWeb/flake.nix` + `.envrc` (`use flake`) | devShell tools loaded by direnv |

VS Code's Go extension is configured via:
- `go.goroot` → Nix home-manager Go path
- `go.alternateTools.go` → `/etc/profiles/per-user/cding/bin/go`

> **Important:** VS Code is launched from Finder/Dock and inherits a bare macOS PATH (`/usr/bin:/bin:/usr/sbin:/sbin`). It does not inherit your shell's mise or Nix PATH. Go resolution inside VS Code depends entirely on `go.alternateTools`, `go.goroot`, and whatever direnv injects via the direnv extension.

---

## Bug 1: Go toolchain version mismatch

### Error message

```
Build Error: go build -o __debug_bin... -gcflags all=-N -l .
# internal/profilerecord
compile: version "go1.25.7" does not match go tool version "go1.25.0"
```

### What it means

The `go` driver binary is version 1.25.0, but it is finding a `compile` binary from Go 1.25.7 (via GOROOT or PATH). Go's compiler and driver must come from the same installation.

### Root cause: Nix flake injecting a conflicting Go via direnv

`ChenWeb/.envrc` runs `use flake`, which activates `ChenWeb/flake.nix`. The flake's `devShells.default` previously listed `go` as a package:

```nix
packages = with pkgs; [
  go          # ← pkgs.go from nixpkgs-unstable = Go 1.25.0 at time of bug
  bun
  just
  ...
];
```

When direnv loads (automatically in VS Code via the direnv extension), it prepends the flake's Go 1.25.0 to PATH. Meanwhile `go.goroot` in VS Code points at Go 1.25.7, so the 1.25.0 `go` driver ends up looking for `compile` in the 1.25.7 GOROOT → mismatch.

**Secondary cause: stale downloaded toolchains**

Go's `GOTOOLCHAIN=auto` (the default) can auto-download toolchains into `$GOMODCACHE/golang.org/toolchain@v0.0.1-goX.Y.Z.darwin-arm64/`. These can also be picked up ahead of the intended Go if PATH lookup finds them first.

> **Nix note:** `GOMODCACHE` is `~/.local/share/go/pkg/mod`, **not** `~/go/pkg/mod`. If you try to clean the toolchain cache, clean the right path.

### Fix

**Step 1 — Remove `go` from the flake** (permanent fix):

In `ChenWeb/flake.nix`, remove `go` from the devShell packages. Go is managed by home-manager and mise; the flake does not need to provide it.

```nix
packages = with pkgs; [
  # go  ← removed; managed by home-manager + mise
  bun
  just
  nodejs_24
  golangci-lint
];
```

Then reload direnv (run `direnv reload` inside the ChenWeb folder, or reload the VS Code window).

**Step 2 — Prevent toolchain auto-download** (one-time):

```bash
go env -w GOTOOLCHAIN=local
```

This writes to `~/.config/go/env` and is respected by every `go` invocation regardless of how it is launched. `local` means: use whichever Go is active; never auto-download a different version.

**Step 3 — Clean stale toolchain downloads** (if needed):

```bash
chmod -R u+w ~/.local/share/go/pkg/mod/golang.org/toolchain@*
rm -rf ~/.local/share/go/pkg/mod/golang.org/toolchain@*
rm -rf ~/.local/share/go/pkg/mod/cache/download/golang.org/toolchain
```

---

## Bug 2: mise.toml Go version is too old after setting GOTOOLCHAIN=local

### Error message

```
go: go.work requires go >= 1.25.0 (running go 1.23.12; GOTOOLCHAIN=local)
```

### What it means

`GOTOOLCHAIN=local` prevents Go from silently downloading a newer version when the active Go is older than what `go.work` requires. Previously this was hidden by `auto` downloading a toolchain on demand.

### Root cause

`ChenWeb/mise.toml` had a stale `go = "1.23.12"` pin. The `go.work` file requires `go 1.25.1`. With `GOTOOLCHAIN=local`, running `mise build-server` from the ChenWeb root activates Go 1.23.12, which is too old to load the workspace.

### Fix

Update `ChenWeb/mise.toml` to match the current Go version:

```toml
[tools]
go = "1.25.7"   # was 1.23.12
```

Keep all subdirectory `mise.toml` files (e.g. `server/cmd/doc-processor/mise.toml`) in sync with the same version.

> **Rule of thumb:** whenever you bump Go in home-manager or nix, also update `ChenWeb/mise.toml` and all `server/cmd/*/mise.toml` files. With `GOTOOLCHAIN=local` there is no automatic fallback.

---

## Bug 3: dlv DAP reverse-mode timeout

### Error message

```
Failed to launch dlv: Error: timed out while waiting for DAP in reverse mode to connect
```

### Root cause

`ChenWeb/.vscode/launch.json` or the workspace-root `.vscode/launch.json` had `"console": "integratedTerminal"`. This forces delve to use **reverse-mode DAP**: VS Code listens on a port and waits for `dlv` to connect back. Every new integrated terminal triggers zsh + mise activation before `dlv` starts. If that is slow enough, `dlv` does not connect within the default timeout.

### Fix

Remove `"console": "integratedTerminal"` from the launch configuration. Debug output goes to VS Code's Debug Console panel (appropriate for a server that does not read stdin).

Check both launch files if VS Code is opened at the workspace root:

- `~/Workspace/.vscode/launch.json`
- `~/Workspace/ChenWeb/.vscode/launch.json`

```json
{
  "name": "Doc Processor",
  "type": "go",
  "request": "launch",
  "mode": "auto",
  "program": "${workspaceFolder}/server/cmd/doc-processor",
  "cwd": "${workspaceFolder}/server/cmd/doc-processor",
  "envFile": "${workspaceFolder}/server/cmd/doc-processor/.env",
  "env": {
    "DOC_PROCESSOR_CONFIG": "${workspaceFolder}/config.toml",
    "MODELS_FILE": "${workspaceFolder}/.models.toml",
    "SHARED_LIB_CONFIG_DIR": "${workspaceFolder}/../shared/libconfig.toml"
  }
}
```

> If stdin is ever needed, keep `"console": "integratedTerminal"` and add `"dlvFlags": ["--init-timeout=120s"]` to buy more time.

---

## Why Nix makes this harder

1. **Multiple Go installations coexist** in the Nix store. A `find / -name go -type f` may return 5+ valid Go binaries at different versions under `/nix/store/...`.

2. **direnv + `use flake` silently prepends to PATH.** The flake's devShell packages take precedence over everything mise or home-manager provides for processes launched in that environment — including VS Code's language server and debug adapter.

3. **VS Code inherits a bare launchd PATH.** PATH inside VS Code is `/usr/bin:/bin:/usr/sbin:/sbin`. Go resolution depends on `go.alternateTools`, `go.goroot`, and direnv — not your shell.

4. **GOMODCACHE is `~/.local/share/go/pkg/mod`**, not `~/go/pkg/mod`, because mise sets `GOPATH=~/.local/share/go`. Cleaning the wrong path has no effect.

5. **`go env -w` is the most reliable way to set Go env vars** across all installations. It writes to `~/.config/go/env`, which every `go` binary reads on startup regardless of GOROOT or installation path.

---

## Quick diagnostic checklist

Run these when debugging breaks with a version mismatch or dlv timeout:

```bash
# 1. What Go does the direnv-loaded ChenWeb environment see?
cd ~/Workspace/ChenWeb && direnv exec . go version
# Expected: go1.25.7 (or current home-manager version)

# 2. Is GOTOOLCHAIN preventing auto-downloads?
go env GOTOOLCHAIN
# Expected: local

# 3. Are there stale downloaded toolchains?
ls ~/.local/share/go/pkg/mod/golang.org/
# Expected: empty (or only "x/")

# 4. Does mise resolve the right Go in the debug directory?
cd ~/Workspace/ChenWeb/server/cmd/doc-processor && mise exec -- go version
# Expected: go1.25.7

# 5. Does a clean build work from the terminal?
cd ~/Workspace/ChenWeb/server/cmd/doc-processor && go build .
# Expected: success, no output
```

All five should agree on the same Go version as home-manager currently provides.

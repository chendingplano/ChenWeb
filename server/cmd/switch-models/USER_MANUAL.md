# Claude Code Model Switcher

Switch Claude Code between Anthropic and DeepSeek models without relying on the cc-switch UI.

## Why

The cc-switch app's UI occasionally fails to persist provider switches — the database and config files don't get updated even though the UI shows success. These scripts update all three config locations directly.

## Usage

```bash
# Switch to Anthropic (official)
./claude-code-switch-to-anthropic

# Switch to DeepSeek
./claude-code-switch-to-deepseek
```

**Restart VS Code** (or your Claude Code session) after switching.

## What Gets Updated

Each script updates three locations:

| File | Change |
|------|--------|
| `~/.cc-switch/settings.json` | `currentProviderClaude` → target provider ID |
| `~/.cc-switch/cc-switch.db` | `is_current` flag on the `providers` table |
| `~/.claude/settings.json` | `env` block — cleared for Anthropic, set to DeepSeek API overrides |

## How It Works

Claude Code reads environment variables from `~/.claude/settings.json` on startup:

- **Anthropic**: empty `env` — Claude Code uses its default authentication (`claude login` or `ANTHROPIC_API_KEY`)
- **DeepSeek**: `env` sets `ANTHROPIC_BASE_URL` to DeepSeek's Anthropic-compatible endpoint and maps model tiers (`haiku` → `deepseek-v4-flash`, `sonnet`/`opus` → `deepseek-v4-pro`)

## Prerequisites

- `python3` (for JSON manipulation)
- `sqlite3` (for database updates)
- cc-switch must be installed with providers already configured

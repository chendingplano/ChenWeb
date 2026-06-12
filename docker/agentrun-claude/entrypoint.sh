#!/bin/sh
# entrypoint.sh — chenweb/agentrun-claude:v1
#
# Reads /workspace/ISSUE.md as the task prompt and invokes Claude Code in
# non-interactive print mode. Stdout and stderr are streamed back to the
# worker by the host-side DockerSandbox.
#
# Required env (injected by the host worker via `docker run --env`):
#   ANTHROPIC_API_KEY (or CLAUDE_API_KEY as an alias)
#
# Exit codes:
#   0  — claude exited successfully
#   2  — /workspace/ISSUE.md missing (worker failed to Prepare)
#   3  — no API key in env
#   *  — pass-through of claude's exit code

set -eu

cd /workspace

if [ ! -f ISSUE.md ]; then
    echo "ERROR: /workspace/ISSUE.md not found" >&2
    exit 2
fi

# Alias CLAUDE_API_KEY → ANTHROPIC_API_KEY if only the former is set, so
# the Claude Code CLI (which reads ANTHROPIC_API_KEY) picks it up.
if [ -z "${ANTHROPIC_API_KEY:-}" ] && [ -n "${CLAUDE_API_KEY:-}" ]; then
    export ANTHROPIC_API_KEY="$CLAUDE_API_KEY"
fi

if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
    echo "ERROR: ANTHROPIC_API_KEY is required (inject via docker run --env)" >&2
    exit 3
fi

# stream-json emits machine-readable events so the ChenWeb worker can
# normalize model usage, tool calls, and the final answer into ap_agent_trace.
# bypassPermissions is acceptable here because the container itself is the
# trust boundary: ephemeral, single-mount /workspace, no other paths accessible.
exec claude \
    --print \
    --output-format stream-json \
    --verbose \
    --permission-mode bypassPermissions \
    --no-session-persistence \
    "$(cat ISSUE.md)"

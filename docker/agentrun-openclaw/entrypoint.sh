#!/bin/sh
# entrypoint.sh — chenweb/agentrun-openclaw:v1
#
# Reads /workspace/ISSUE.md as the task prompt and invokes the OpenClaw
# CLI in non-interactive mode. Stdout and stderr are streamed back to
# the worker by the host-side DockerSandbox.
#
# Required env (injected by the host worker via `docker run --env`):
#   ANTHROPIC_API_KEY (or CLAUDE_API_KEY as an alias)
#
# Exit codes:
#   0  — openclaw exited successfully
#   2  — /workspace/ISSUE.md missing (worker failed to Prepare)
#   3  — no API key in env
#   *  — pass-through of openclaw's exit code
#
# VERIFY: the CLI invocation below is a best-guess non-interactive form.
# OpenClaw's released CLI flags are maintainer-dependent; pin the image
# with OPENCLAW_VERSION at build or override AGENT_PLATFORM_OPENCLAW_IMAGE
# if the flags change.

set -eu

cd /workspace

if [ ! -f ISSUE.md ]; then
    echo "ERROR: /workspace/ISSUE.md not found" >&2
    exit 2
fi

# Alias CLAUDE_API_KEY → ANTHROPIC_API_KEY if only the former is set,
# since OpenClaw speaks to Anthropic and most SDKs read the Anthropic var.
if [ -z "${ANTHROPIC_API_KEY:-}" ] && [ -n "${CLAUDE_API_KEY:-}" ]; then
    export ANTHROPIC_API_KEY="$CLAUDE_API_KEY"
fi

if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
    echo "ERROR: ANTHROPIC_API_KEY is required (inject via docker run --env)" >&2
    exit 3
fi

# Non-interactive print-mode invocation. Trust boundary is the container
# itself (ephemeral FS, single bind mount, host-enforced caps), so auto-
# apply is acceptable.
exec openclaw --print --yes "$(cat ISSUE.md)"

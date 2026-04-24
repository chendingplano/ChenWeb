#!/bin/sh
# entrypoint.sh — chenweb/agentrun-codex:v1
#
# Reads /workspace/ISSUE.md as the task prompt and invokes OpenAI's Codex
# CLI in non-interactive execution mode. Stdout and stderr are streamed
# back to the worker by the host-side DockerSandbox.
#
# Required env (injected by the host worker via `docker run --env`):
#   OPENAI_API_KEY
#
# Exit codes:
#   0  — codex exited successfully
#   2  — /workspace/ISSUE.md missing (worker failed to Prepare)
#   3  — no API key in env
#   *  — pass-through of codex's exit code
#
# VERIFY: the flags below match the documented non-interactive form of
# @openai/codex at image build time. If the CLI evolves, pin
# CODEX_VERSION at build or override AGENT_PLATFORM_CODEX_IMAGE to a
# tag that retests the invocation.

set -eu

cd /workspace

if [ ! -f ISSUE.md ]; then
    echo "ERROR: /workspace/ISSUE.md not found" >&2
    exit 2
fi

if [ -z "${OPENAI_API_KEY:-}" ]; then
    echo "ERROR: OPENAI_API_KEY is required (inject via docker run --env)" >&2
    exit 3
fi

# `codex exec` is the non-interactive subcommand; --full-auto approves
# file edits without prompting. The container is the trust boundary
# (ephemeral FS, single bind mount, host-enforced caps), so auto-apply
# is acceptable.
exec codex exec --full-auto "$(cat ISSUE.md)"

#!/bin/sh
# entrypoint.sh — chenweb/agentrun-opencode:v1
#
# Reads /workspace/ISSUE.md as the task prompt and invokes SST's
# OpenCode CLI in non-interactive `run` mode. Stdout and stderr are
# streamed back to the worker by the host-side DockerSandbox.
#
# Required env (injected by the host worker via `docker run --env`):
#   OPENAI_API_KEY   — default provider. OpenCode supports other
#                      providers too (Anthropic, Gemini, etc.); add
#                      their keys to the host env and ChenWeb's
#                      envSecretsForKind() to forward them here.
#
# Exit codes:
#   0  — opencode exited successfully
#   2  — /workspace/ISSUE.md missing (worker failed to Prepare)
#   3  — no provider key in env
#   *  — pass-through of opencode's exit code
#
# VERIFY: the `opencode run` subcommand and --yes auto-approve flag are
# the documented non-interactive surface at image build time. If the
# CLI evolves, pin OPENCODE_VERSION at build or override
# AGENT_PLATFORM_OPENCODE_IMAGE to a retested tag.

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

# `opencode run <prompt>` is the non-interactive subcommand; --yes
# auto-approves edits. Trust boundary is the container itself.
exec opencode run --yes "$(cat ISSUE.md)"

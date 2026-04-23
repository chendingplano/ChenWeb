# chenweb/agentrun-claude:v1
#
# Sandbox image for executing Anthropic's Claude Code CLI on behalf of
# ChenWeb's agent-platform workers (M1c). Reads /workspace/ISSUE.md as
# the prompt and lets Claude Code edit files in /workspace.
#
# Build:   docker build -t chenweb/agentrun-claude:v1 .
# Runtime: launched by shared/go/api/llm/agentrun/runner_claude.go
#          via DockerSandbox.Run. Expects ANTHROPIC_API_KEY (or
#          CLAUDE_API_KEY) in the container env.
#
# No secrets are baked into this image. The caller supplies keys per-run
# via --env.

FROM node:20-slim

ARG CLAUDE_CODE_VERSION=latest

# Install git + ca-certs so Claude Code can work in a scratch workdir
# that may contain a git repo, and so HTTPS to api.anthropic.com works.
RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION} \
    && npm cache clean --force

# Copy the entrypoint, then drop to a non-root user. Root inside the
# container is mitigated by --network=bridge isolation and the host-level
# memory/cpu limits, but there's no reason to run the agent as root.
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

RUN useradd -m -s /bin/bash agent
USER agent
WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]

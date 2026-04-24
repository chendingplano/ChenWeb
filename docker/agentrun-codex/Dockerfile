# chenweb/agentrun-codex:v1
#
# Sandbox image for executing OpenAI's Codex CLI on behalf of ChenWeb's
# agent-platform workers (M3). Reads /workspace/ISSUE.md as the prompt
# and lets Codex edit files in /workspace.
#
# Build:   docker build -t chenweb/agentrun-codex:v1 .
# Runtime: launched by shared/go/api/llm/agentrun/runner_codex.go via
#          DockerSandbox.Run. Expects OPENAI_API_KEY in the container env.
#
# No secrets are baked into this image. The caller supplies keys per-run
# via --env.

FROM node:20-slim

ARG CODEX_VERSION=latest

# git + ca-certs: Codex may init or inspect a repo; HTTPS is required for
# api.openai.com.
RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g @openai/codex@${CODEX_VERSION} \
    && npm cache clean --force

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

RUN useradd -m -s /bin/bash agent
USER agent
WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]

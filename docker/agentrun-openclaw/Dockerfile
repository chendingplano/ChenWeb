# chenweb/agentrun-openclaw:v1
#
# Sandbox image for executing the OpenClaw CLI on behalf of ChenWeb's
# agent-platform workers (M3). Reads /workspace/ISSUE.md as the prompt
# and lets OpenClaw edit files in /workspace.
#
# Build:   docker build -t chenweb/agentrun-openclaw:v1 .
# Runtime: launched by shared/go/api/llm/agentrun/runner_openclaw.go via
#          DockerSandbox.Run. Expects ANTHROPIC_API_KEY (or
#          CLAUDE_API_KEY) in the container env.
#
# No secrets are baked into this image. The caller supplies keys per-run
# via --env.
#
# VERIFY: OPENCLAW_PACKAGE below is a best-guess npm identifier. Confirm
# against the maintainer's release channel and override the build-arg if
# needed.

FROM node:20-slim

ARG OPENCLAW_PACKAGE=openclaw
ARG OPENCLAW_VERSION=latest

# git + ca-certs: OpenClaw may init/inspect a git repo; HTTPS is required
# for api.anthropic.com.
RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g ${OPENCLAW_PACKAGE}@${OPENCLAW_VERSION} \
    && npm cache clean --force

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

RUN useradd -m -s /bin/bash agent
USER agent
WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]

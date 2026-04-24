# chenweb/agentrun-opencode:v1
#
# Sandbox image for executing SST's OpenCode CLI (https://opencode.ai)
# on behalf of ChenWeb's agent-platform workers (M3). Reads
# /workspace/ISSUE.md as the prompt and lets OpenCode edit files in
# /workspace.
#
# Build:   docker build -t chenweb/agentrun-opencode:v1 .
# Runtime: launched by shared/go/api/llm/agentrun/runner_opencode.go via
#          DockerSandbox.Run. Expects OPENAI_API_KEY in the container env.
#
# No secrets are baked into this image. The caller supplies keys per-run
# via --env.

FROM node:20-slim

ARG OPENCODE_PACKAGE=opencode-ai
ARG OPENCODE_VERSION=latest

# git + ca-certs: OpenCode may init/inspect a git repo; HTTPS is required
# for provider APIs (OpenAI by default; user can route to other
# providers via their own OpenCode config).
RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g ${OPENCODE_PACKAGE}@${OPENCODE_VERSION} \
    && npm cache clean --force

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

RUN useradd -m -s /bin/bash agent
USER agent
WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]

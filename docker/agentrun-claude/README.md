# agentrun-claude

Sandbox container image for ChenWeb's agent platform (M1c).

The host-side runner `ClaudeCodeRunner` at
[`shared/go/api/llm/agentrun/runner_claude.go`](../../../shared/go/api/llm/agentrun/runner_claude.go)
launches this image once per `ap_task_run` row whose assigned agent has
`runtime_kind = 'claude_code'`. The container reads `/workspace/ISSUE.md`,
invokes the Claude Code CLI in `--print --output-format stream-json --verbose`
mode, and lets it edit files in `/workspace`. The JSONL stdout stream is
normalized by ChenWeb into `ap_agent_trace` and exported as OpenTelemetry span
data for HyperDX. Host-side `Collect` then walks the workdir for artifacts.

## Build

```sh
cd ChenWeb/docker/agentrun-claude
docker build -t chenweb/agentrun-claude:v1 .
```

Pin a specific Claude Code version:

```sh
docker build --build-arg CLAUDE_CODE_VERSION=1.2.3 -t chenweb/agentrun-claude:v1 .
```

Override the tag the runner uses at runtime without rebuilding the
server by setting `AGENT_PLATFORM_CLAUDE_IMAGE=my-registry/claude:custom`.

## Smoke test (outside ChenWeb)

```sh
mkdir -p /tmp/claude-test
printf "# Say hi\n\nCreate hello.txt with 'hello world' in it." \
    > /tmp/claude-test/ISSUE.md

docker run --rm \
    --network=bridge \
    -e ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" \
    -v /tmp/claude-test:/workspace:rw \
    chenweb/agentrun-claude:v1
```

You should see Claude Code's streamed output and a new `hello.txt` appear
under `/tmp/claude-test/`.

## Runtime contract

| Piece | Provided by |
|---|---|
| `/workspace/ISSUE.md` | Host worker's `ClaudeCodeRunner.Prepare` |
| `/workspace/*` (output) | The agent; read back by `ClaudeCodeRunner.Collect` |
| `ANTHROPIC_API_KEY` | Server env → passed via `docker run --env` |
| `--network=bridge` | `DockerSandbox` (agent must reach `api.anthropic.com`) |
| `--memory 2048m --cpus 2` | `DockerSandbox` defaults |
| Lifetime | Host worker enforces a 15-minute ctx timeout |

## Security notes

- The image runs as a non-root user (`agent`).
- `--permission-mode bypassPermissions` is set in the entrypoint because the
  container boundary is the trust boundary: ephemeral FS, a single bind
  mount, network-scoped via Docker bridge, host-enforced CPU/memory caps.
- No secrets are baked into the image. API keys arrive per-run via
  `--env` and never hit disk inside the container.
- Host workdirs (`<AGENT_PLATFORM_WORKDIR_ROOT>/<run_id>/`) are removed on
  success by the worker. Failed runs keep the workdir for debugging.

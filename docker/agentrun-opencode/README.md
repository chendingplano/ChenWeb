# agentrun-opencode

Sandbox container image for ChenWeb's agent platform (M3).

The host-side runner `OpenCodeRunner` at
[`shared/go/api/llm/agentrun/runner_opencode.go`](../../../shared/go/api/llm/agentrun/runner_opencode.go)
launches this image once per `ap_task_run` row whose assigned agent has
`runtime_kind = 'opencode'`. The container reads `/workspace/ISSUE.md`,
invokes `opencode run --yes`, and lets it edit files in `/workspace`.
Host-side `Collect` then walks the workdir for artifacts.

## Build

```sh
cd ChenWeb/docker/agentrun-opencode
docker build -t chenweb/agentrun-opencode:v1 .
```

Pin a specific OpenCode version:

```sh
docker build --build-arg OPENCODE_VERSION=0.3.0 -t chenweb/agentrun-opencode:v1 .
```

If SST repackages under a different npm identifier, override
`OPENCODE_PACKAGE` too:

```sh
docker build \
    --build-arg OPENCODE_PACKAGE=@sst/opencode \
    --build-arg OPENCODE_VERSION=0.3.0 \
    -t chenweb/agentrun-opencode:v1 .
```

Override the tag the runner uses at runtime without rebuilding the
server by setting `AGENT_PLATFORM_OPENCODE_IMAGE=my-registry/opencode:custom`.

## Smoke test (outside ChenWeb)

```sh
mkdir -p /tmp/opencode-test
printf "# Say hi\n\nCreate hello.txt containing 'hello world'." \
    > /tmp/opencode-test/ISSUE.md

docker run --rm \
    --network=bridge \
    -e OPENAI_API_KEY="$OPENAI_API_KEY" \
    -v /tmp/opencode-test:/workspace:rw \
    chenweb/agentrun-opencode:v1
```

You should see OpenCode's streamed output and a new `hello.txt` appear
under `/tmp/opencode-test/`.

## Runtime contract

| Piece | Provided by |
|---|---|
| `/workspace/ISSUE.md` | Host worker's `OpenCodeRunner.Prepare` |
| `/workspace/*` (output) | The agent; read back by `OpenCodeRunner.Collect` |
| `OPENAI_API_KEY` | Server env → passed via `docker run --env` |
| `--network=bridge` | `DockerSandbox` (agent must reach provider APIs) |
| `--memory 2048m --cpus 2` | `DockerSandbox` defaults |
| Lifetime | Host worker enforces a 15-minute ctx timeout |

## Provider flexibility

OpenCode routes to multiple providers (OpenAI, Anthropic, Gemini, etc.)
based on its own config. The worker's `envSecretsForKind("opencode")` in
[`worker.go`](../../server/api/agentplatformhandler/worker.go)
currently forwards only `OPENAI_API_KEY`. To use a different provider
inside the container:

1. Add the provider's env var name to that switch case.
2. Rebuild the server.
3. Set the new env var at process start.

## Security notes

- The image runs as a non-root user (`agent`).
- `--yes` is set in the entrypoint because the container boundary is
  the trust boundary: ephemeral FS, a single bind mount, network scoped
  via Docker bridge, host-enforced CPU/memory caps.
- No secrets are baked into the image. API keys arrive per-run via
  `--env` and never hit disk inside the container.
- Host workdirs (`<AGENT_PLATFORM_WORKDIR_ROOT>/<run_id>/`) are removed on
  success by the worker. Failed runs keep the workdir for debugging.

## Verification

The `opencode run --yes "<prompt>"` invocation in
[`entrypoint.sh`](entrypoint.sh) matches the documented non-interactive
surface at image build time. If the CLI changes, pin `OPENCODE_VERSION`
at build, retest, and rebuild. `AGENT_PLATFORM_OPENCODE_IMAGE` lets
operators swap images without touching the server binary.

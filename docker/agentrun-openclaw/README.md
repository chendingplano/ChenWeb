# agentrun-openclaw

Sandbox container image for ChenWeb's agent platform (M3).

The host-side runner `OpenClawRunner` at
[`shared/go/api/llm/agentrun/runner_openclaw.go`](../../../shared/go/api/llm/agentrun/runner_openclaw.go)
launches this image once per `ap_task_run` row whose assigned agent has
`runtime_kind = 'openclaw'`. The container reads `/workspace/ISSUE.md`,
invokes the OpenClaw CLI in non-interactive mode, and lets it edit
files in `/workspace`. Host-side `Collect` then walks the workdir for
artifacts.

## Build

```sh
cd ChenWeb/docker/agentrun-openclaw
docker build -t chenweb/agentrun-openclaw:v1 .
```

If your OpenClaw npm package name or version differs:

```sh
docker build \
    --build-arg OPENCLAW_PACKAGE=@scope/openclaw \
    --build-arg OPENCLAW_VERSION=1.2.3 \
    -t chenweb/agentrun-openclaw:v1 .
```

Override the tag the runner uses at runtime without rebuilding the
server by setting `AGENT_PLATFORM_OPENCLAW_IMAGE=my-registry/openclaw:custom`.

## Smoke test (outside ChenWeb)

```sh
mkdir -p /tmp/openclaw-test
printf "# Say hi\n\nCreate hello.txt containing 'hello world'." \
    > /tmp/openclaw-test/ISSUE.md

docker run --rm \
    --network=bridge \
    -e ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" \
    -v /tmp/openclaw-test:/workspace:rw \
    chenweb/agentrun-openclaw:v1
```

You should see OpenClaw's streamed output and a new `hello.txt` appear
under `/tmp/openclaw-test/`.

## Runtime contract

| Piece | Provided by |
|---|---|
| `/workspace/ISSUE.md` | Host worker's `OpenClawRunner.Prepare` |
| `/workspace/*` (output) | The agent; read back by `OpenClawRunner.Collect` |
| `ANTHROPIC_API_KEY` | Server env → passed via `docker run --env` |
| `--network=bridge` | `DockerSandbox` (agent must reach `api.anthropic.com`) |
| `--memory 2048m --cpus 2` | `DockerSandbox` defaults |
| Lifetime | Host worker enforces a 15-minute ctx timeout |

## Security notes

- The image runs as a non-root user (`agent`).
- The entrypoint runs the CLI in auto-approval mode because the
  container boundary is the trust boundary: ephemeral FS, a single
  bind mount, network scoped via Docker bridge, host-enforced CPU/memory
  caps.
- No secrets are baked into the image. API keys arrive per-run via
  `--env` and never hit disk inside the container.
- Host workdirs (`<AGENT_PLATFORM_WORKDIR_ROOT>/<run_id>/`) are removed on
  success by the worker. Failed runs keep the workdir for debugging.

## Verification

Both the npm package identifier (`OPENCLAW_PACKAGE`, default `openclaw`)
and the CLI invocation in [`entrypoint.sh`](entrypoint.sh) are best-guess
defaults. OpenClaw is maintainer-versioned; confirm both against the
release you intend to ship before rolling to production. Use the
`AGENT_PLATFORM_OPENCLAW_IMAGE` env var to swap images without touching
the server binary.

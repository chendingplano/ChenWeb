# agentrun-codex

Sandbox container image for ChenWeb's agent platform (M3).

The host-side runner `CodexRunner` at
[`shared/go/api/llm/agentrun/runner_codex.go`](../../../shared/go/api/llm/agentrun/runner_codex.go)
launches this image once per `ap_task_run` row whose assigned agent has
`runtime_kind = 'codex'`. The container reads `/workspace/ISSUE.md`,
invokes `codex exec --full-auto`, and lets it edit files in `/workspace`.
Host-side `Collect` then walks the workdir for artifacts.

## Build

```sh
cd ChenWeb/docker/agentrun-codex
docker build -t chenweb/agentrun-codex:v1 .
```

Pin a specific Codex CLI version:

```sh
docker build --build-arg CODEX_VERSION=0.10.0 -t chenweb/agentrun-codex:v1 .
```

Override the tag the runner uses at runtime without rebuilding the
server by setting `AGENT_PLATFORM_CODEX_IMAGE=my-registry/codex:custom`.

## Smoke test (outside ChenWeb)

```sh
mkdir -p /tmp/codex-test
printf "# Say hi\n\nCreate hello.txt containing 'hello world'." \
    > /tmp/codex-test/ISSUE.md

docker run --rm \
    --network=bridge \
    -e OPENAI_API_KEY="$OPENAI_API_KEY" \
    -v /tmp/codex-test:/workspace:rw \
    chenweb/agentrun-codex:v1
```

You should see Codex's streamed output and a new `hello.txt` appear
under `/tmp/codex-test/`.

## Runtime contract

| Piece | Provided by |
|---|---|
| `/workspace/ISSUE.md` | Host worker's `CodexRunner.Prepare` |
| `/workspace/*` (output) | The agent; read back by `CodexRunner.Collect` |
| `OPENAI_API_KEY` | Server env → passed via `docker run --env` |
| `--network=bridge` | `DockerSandbox` (agent must reach `api.openai.com`) |
| `--memory 2048m --cpus 2` | `DockerSandbox` defaults |
| Lifetime | Host worker enforces a 15-minute ctx timeout |

## Security notes

- The image runs as a non-root user (`agent`).
- `--full-auto` is set in the entrypoint because the container boundary
  is the trust boundary: ephemeral FS, a single bind mount, network
  scoped via Docker bridge, host-enforced CPU/memory caps.
- No secrets are baked into the image. API keys arrive per-run via
  `--env` and never hit disk inside the container.
- Host workdirs (`<AGENT_PLATFORM_WORKDIR_ROOT>/<run_id>/`) are removed on
  success by the worker. Failed runs keep the workdir for debugging.

## Verification

The `codex exec --full-auto "<prompt>"` invocation in
[`entrypoint.sh`](entrypoint.sh) is the documented non-interactive form
at image build time. If an upgraded CLI changes this surface, pin
`CODEX_VERSION` at build, retest, and rebuild. Operators can override
the image tag via `AGENT_PLATFORM_CODEX_IMAGE` without touching the
server binary.

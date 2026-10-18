# MinerU 4 CPU container on onto

The host is Ubuntu 18.04/glibc 2.27. PyTorch 2.7 CPU wheels require glibc 2.28;
this image uses an existing, digest-pinned Python 3.12/Bullseye base with glibc
2.31. It installs PyTorch 2.7.1+cpu, torchvision 0.22.1+cpu and the workspace's
MinerU 4.0.9 source. OpenCV uses its headless distribution; only that dependency
name is changed in the staged MinerU package metadata.

The image snapshots the deployed PDF worker and replaces its MinerU backend,
result adapter and table repair module with the workspace's MinerU 4 versions.
The September worker's OCR fallback call and mode detection are adjusted to the
MinerU 4 basic OCR interface. Its database and JetStream code stays at the deployed
version. No `.env`, documents or model weights are baked into the image.

## Build

From the Mac's workspace root, upload these container files and the three
adapters (the native deployment's Python files remain available):

```sh
rsync -az -e 'ssh -p 8822' ChenWeb/python/pdf-parser/container/ \
  gui@210.5.158.91:Workspace/ChenWeb/python/pdf-parser/container/
rsync -az -e 'ssh -p 8822' \
  ChenWeb/python/pdf-parser/parser_mineru.py \
  ChenWeb/python/pdf-parser/mineru_v4_adapter.py \
  ChenWeb/python/pdf-parser/mineru_table_repair.py \
  gui@210.5.158.91:Workspace/ChenWeb/python/pdf-parser/container/mineru-v4/
```

On onto, choose an empty build directory. The MinerU source directory must
already contain version 4.0.9, including its `resources/` package assets:

```sh
cd /home/gui/Workspace/ChenWeb/python/pdf-parser
python3 container/prepare-context.py \
  --worker . --adapters container/mineru-v4 \
  --mineru /home/gui/Workspace/ThirdParty/mineru \
  --output /home/gui/Workspace/deploy/mineru-4.0.9-build
cd /home/gui/Workspace/deploy/mineru-4.0.9-build
docker build --network=host -t chenweb-pdf-parser:mineru-4.0.9 .
docker run --rm chenweb-pdf-parser:mineru-4.0.9 python -m pip check
docker run --rm chenweb-pdf-parser:mineru-4.0.9 \
  python -c 'import torch; print(torch.__version__); print(torch.ones(2,2) @ torch.ones(2,2))'
```

## Models and runtime

Create `/home/gui/.mineru` and `/home/gui/.cache` owned by `gui`. Install
`mineru.yaml` as `/home/gui/.mineru/config.yaml`, preserving any existing user
configuration before replacing it. Download and verify models using the image:

```sh
docker run --rm --network host --user 1001:1001 \
  -v /home/gui/.mineru:/home/gui/.mineru \
  -v /home/gui/.cache:/home/gui/.cache \
  chenweb-pdf-parser:mineru-4.0.9 \
  mineru-kit models download --tier standard --source modelscope
docker run --rm --user 1001:1001 \
  -v /home/gui/.mineru:/home/gui/.mineru \
  chenweb-pdf-parser:mineru-4.0.9 \
  mineru-kit models verify --tier standard
```

Models use ONNX for layout/OCR/formula processing (MinerU's default CPU backend)
and llama.cpp with GGUF weights for the standard tier's VLM. PyTorch 2.7.1 CPU is
installed and works independently; importing it alongside mineru-llama-cpp 0.1.3
in one process crashes in this image, so the production CPU parser uses ONNX.
Both model bundles persist under
`/home/gui/.mineru/models`. Runtime model source is `local`, so a missing model
fails visibly rather than starting a background download during a PDF job.

As root, install `pdf-parser-container.conf` at
`/etc/systemd/system/pdf-parser.service.d/container.conf`, run
`systemctl daemon-reload`, then `systemctl restart pdf-parser`. The original unit
remains in place. The drop-in changes its command to `run-container.py` and sets
`MINERU_OCR_MODE=ocr` to preserve the deployment's previous `-m ocr` behavior.

The launcher forwards systemd's parsed environment by variable name, preserving
the existing `.env` quoting rules without writing credentials into another file
or command line. It excludes native Python/MinerU path settings so the container
uses its own installed runtime. Docker uses host networking for the existing
loopback PostgreSQL and NATS connections and runs as `gui` (1001:1001). Workspace,
model and cache directories are bind-mounted at their existing absolute paths;
PDF results therefore remain accessible to all downstream services.

```sh
systemctl status pdf-parser
docker inspect chenweb-pdf-parser --format '{{.State.Status}} {{.Config.Image}}'
docker exec chenweb-pdf-parser mineru-kit --version
docker exec chenweb-pdf-parser python -c 'import torch; print(torch.__version__)'
journalctl -u pdf-parser -n 40 --no-pager
```

Systemd owns startup and restart; the container has no independent restart
policy. `docker stop` gets 60 seconds for shutdown, followed by container cleanup.
To restore the previous unit command, remove only the `container.conf` drop-in,
reload systemd and restart the unit. This restores the previous command and
requires its native MinerU environment to be functional; it does not repair the
incomplete native 4.0.9 installation from the earlier upgrade attempt.

## Verified deployment

On 2026-10-18, standalone container parses passed for a GBZ 221-2009 cover page
in standard, forced standard OCR and basic OCR modes. A table page produced two
HTML tables and two image assets, with every table image reference checked on
disk. The smoke runner also checks page count, useful text and the saved content
list without claiming a database record or publishing a JetStream event.

The live unit runs as `gui`, has `MINERU_OCR_MODE=ocr`, and reconnects to the
existing `pdf-parser` durable consumer. Model verification and `pip check` pass.
Build logs, model logs, installed package versions and smoke results are retained
under `/home/gui/Workspace/deploy/mineru-4.0.9-container/`.

For another sample already under the mounted workspace, the smoke script accepts
an input PDF and output directory; `--ocr` selects the basic OCR fallback:

```sh
docker exec chenweb-pdf-parser python \
  /home/gui/Workspace/ChenWeb/python/pdf-parser/container/smoke.py \
  /home/gui/Workspace/deploy/mineru-4.0.9-container/smoke/input.pdf \
  /home/gui/Workspace/deploy/mineru-4.0.9-container/smoke/recheck
```

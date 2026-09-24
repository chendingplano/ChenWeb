"""OpenDataLoader PDF parser backend.

Invokes the opendataloader-pdf Java CLI as a subprocess.
"""

import glob
import json
import logging
import os
import shutil
import subprocess
import time
from typing import Any, Callable

from parser_base import ParserBackend

log = logging.getLogger(__name__)

_DEFAULT_JAR_PATHS = [
    os.path.expanduser(
        "~/Workspace/ThirdParty/opendataloader-pdf/python/opendataloader-pdf"
        "/src/opendataloader_pdf/jar/opendataloader-pdf-cli.jar"
    ),
]


def _env(key: str, default: str = "") -> str:
    return os.environ.get(key, default).strip()


class OpenDataParser(ParserBackend):
    name = "opendata"

    def __init__(self) -> None:
        self._jar_path: str = ""
        self._initialized: bool = False

    def init(self) -> None:
        if self._initialized:
            return

        if not shutil.which("java"):
            raise RuntimeError(
                "OpenDataParser requires java on PATH. "
                "Install Java 11+ and ensure 'java' is accessible."
            )

        jar = _env("OPENDATA_JAR_PATH")
        if jar and os.path.isfile(jar):
            self._jar_path = jar
        else:
            for candidate in _DEFAULT_JAR_PATHS:
                if os.path.isfile(candidate):
                    self._jar_path = candidate
                    break

        if not self._jar_path:
            raise RuntimeError(
                "opendataloader-pdf-cli.jar not found. "
                "Set OPENDATA_JAR_PATH or install opendataloader-pdf."
            )

        self._initialized = True
        log.info("OpenDataParser initialized: jar=%s", self._jar_path)

    def parse(
        self,
        pdf_path: str,
        output_dir: str,
        on_progress: Callable[[int, int], None],
    ) -> dict[str, Any]:
        on_progress(0, 1)

        cmd = [
            "java", "-jar", self._jar_path,
            pdf_path,
            "--output-dir", output_dir,
            "--format", "markdown,json",
            "--quiet",
        ]
        log.info("running opendataloader: %s", " ".join(cmd))

        # subprocess.run() blocks with no checkpoint until the whole CLI exits,
        # so an abort request could never be noticed until the parse already
        # finished on its own. Poll instead, re-checking on_progress (which
        # raises when the user aborts) so the subprocess can be killed promptly.
        proc = subprocess.Popen(
            cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
        )
        stop_exc: list[BaseException] = []
        while proc.poll() is None:
            try:
                on_progress(0, 1)
            except Exception as exc:
                stop_exc.append(exc)
                proc.terminate()
                try:
                    proc.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait()
                break
            time.sleep(1.0)

        rc = proc.wait()
        output = proc.stdout.read() if proc.stdout else ""
        if stop_exc:
            raise stop_exc[0]
        if rc != 0:
            raise RuntimeError(f"opendataloader exited {rc}: {output}")

        json_files = glob.glob(os.path.join(output_dir, "*.json"))
        if not json_files:
            raise FileNotFoundError(f"No JSON output in {output_dir}")

        with open(json_files[0], "r", encoding="utf-8") as f:
            raw_result = json.load(f)

        total_pages = raw_result.get("number of pages", 0)

        on_progress(1, 1)
        log.info("opendataloader finished: pages=%d", total_pages)

        return {
            "pages": raw_result.get("kids", []),
            "total_pages": total_pages,
            "engine": "opendata",
            "raw": raw_result,
        }

#!/usr/bin/env python3
"""Forward systemd's parsed environment to the PDF parser container."""

import os

excluded = {
    "HOME", "PATH", "USER", "LOGNAME", "SHELL", "VIRTUAL_ENV", "PYTHONPATH",
    "MINERU_CLI", "MINERU_PYTHON", "MINERU_CONFIG", "MINERU_BACKEND",
    "MINERU_TOOLS_CONFIG_JSON", "MINERU_EXTRA_ARGS", "MINERU_DEVICE_MODE",
    "MINERU_MODEL_SMALL_BACKEND", "MINERU_MODEL_VLM_ENGINE",
    "NOTIFY_SOCKET", "JOURNAL_STREAM", "INVOCATION_ID",
}
args = [
    "/usr/bin/docker", "run", "--rm", "--init", "--name", "chenweb-pdf-parser",
    "--network", "host", "--user", f"{os.getuid()}:{os.getgid()}",
    "--mount", "type=bind,src=/home/gui/Workspace,dst=/home/gui/Workspace",
    "--mount", "type=bind,src=/home/gui/.mineru,dst=/home/gui/.mineru",
    "--mount", "type=bind,src=/home/gui/.cache,dst=/home/gui/.cache",
]
for key in sorted(os.environ.keys() - excluded):
    args += ["--env", key]
args += ["chenweb-pdf-parser:mineru-4.0.9"]
os.execv(args[0], args)

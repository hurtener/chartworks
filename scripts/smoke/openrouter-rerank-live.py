#!/usr/bin/env python3
"""Run one opt-in paid rerank through Chartworks' Bifrost gateway.

Usage: python3 scripts/smoke/openrouter-rerank-live.py --env-file /private/path/.env
The file is read as data, never sourced as shell code. No credential is printed.
"""

import argparse
import os
from pathlib import Path
import subprocess
import sys


def variables(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        name, value = line.split("=", 1)
        name = name.strip()
        if name not in {"OPENROUTER_API_KEY", "RERANK_MODEL"}:
            continue
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
            value = value[1:-1]
        elif " #" in value:
            value = value.split(" #", 1)[0].rstrip()
        values[name] = value
    return values


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--env-file", type=Path, help="private root .env path")
    args = parser.parse_args()
    from_file = variables(args.env_file) if args.env_file else {}
    key = os.environ.get("CHARTWORKS_OPENROUTER_API_KEY") or os.environ.get("OPENROUTER_API_KEY") or from_file.get("OPENROUTER_API_KEY")
    model = os.environ.get("RERANK_MODEL") or from_file.get("RERANK_MODEL")
    if not key:
        parser.error("OpenRouter key is missing")
    if model and model != "cohere/rerank-4-fast":
        parser.error("RERANK_MODEL must be cohere/rerank-4-fast for this smoke")
    env = os.environ.copy()
    env["CHARTWORKS_OPENROUTER_API_KEY"] = key
    env["CHARTWORKS_LIVE_OPENROUTER_RERANK"] = "1"
    return subprocess.run(
        ["go", "test", "./internal/gateway/bifrost", "-run", "^TestLiveOpenRouterRerank$", "-count=1", "-v"],
        cwd=Path(__file__).resolve().parents[2],
        env=env,
        check=False,
    ).returncode


if __name__ == "__main__":
    sys.exit(main())

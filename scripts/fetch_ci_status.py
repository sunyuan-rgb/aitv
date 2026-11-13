#!/usr/bin/env python3
"""导出最近 CI 运行状态到 docs/testing/ci_runs.txt（curl 实现，凭据不落盘/不输出）。"""
import json
import os
import subprocess
import tempfile
from pathlib import Path

REPO = "sunyuan-rgb/aitv"
OUT = Path("docs/testing/ci_runs.txt")
API = f"https://api.github.com/repos/{REPO}/actions/runs?per_page=10"


def main() -> int:
    p = subprocess.run(
        ["git", "credential", "fill"],
        input="protocol=https\nhost=github.com\n\n",
        capture_output=True, text=True,
    )
    tokens = [ln[len("password="):] for ln in p.stdout.splitlines() if ln.startswith("password=")]
    if not tokens:
        print("未能获取 GitHub 凭据")
        return 1

    fd, cfg = tempfile.mkstemp(suffix=".curlrc")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            f.write(f'header = "Authorization: token {tokens[0]}"\n')
            f.write('header = "User-Agent: aitv-dev"\n')
            f.write('header = "Accept: application/vnd.github+json"\n')
        r = subprocess.run(["curl", "-s", "-K", cfg, API],
                           capture_output=True, text=True, encoding="utf-8")
    finally:
        os.remove(cfg)

    data = json.loads(r.stdout or "{}")
    lines = []
    for run in data.get("workflow_runs", []):
        lines.append(
            f"{run['name']} | {run['head_branch']} | {run['head_sha'][:7]} | {run['event']} | "
            f"{run['status']} | {run['conclusion']} | {run['html_url']}"
        )
    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print("\n".join(lines))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

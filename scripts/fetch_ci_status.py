#!/usr/bin/env python3
"""导出最近 CI 运行状态到 docs/testing/ci_runs.txt（凭据取自 git credential fill，不输出凭据）。"""
import json
import subprocess
import urllib.request
from pathlib import Path

REPO = "sunyuan-rgb/aitv"
OUT = Path("docs/testing/ci_runs.txt")


def main() -> int:
    p = subprocess.run(
        ["git", "credential", "fill"],
        input="protocol=https\nhost=github.com\n\n",
        capture_output=True,
        text=True,
    )
    tokens = [ln[len("password="):] for ln in p.stdout.splitlines() if ln.startswith("password=")]
    if not tokens:
        print("未能获取 GitHub 凭据")
        return 1
    req = urllib.request.Request(
        f"https://api.github.com/repos/{REPO}/actions/runs?per_page=10",
        headers={"Authorization": f"token {tokens[0]}", "User-Agent": "aitv-dev"},
    )
    data = json.load(urllib.request.urlopen(req, timeout=30))
    lines = []
    for r in data.get("workflow_runs", []):
        lines.append(
            f"{r['name']} | {r['head_branch']} | {r['head_sha'][:7]} | {r['event']} | "
            f"{r['status']} | {r['conclusion']} | {r['html_url']}"
        )
    OUT.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print("\n".join(lines))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3
"""用 curl 读取 main 分支保护并保存原始结果（凭据不落盘、不输出）。"""
import json
import os
import subprocess
import tempfile
from pathlib import Path

REPO = "sunyuan-rgb/aitv"
OUT = Path("docs/testing/branch_protection.json")
API = f"https://api.github.com/repos/{REPO}/branches/main/protection"


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
        r = subprocess.run(
            ["curl", "-s", "-K", cfg, "-w", "\n%{http_code}", API],
            capture_output=True, text=True, encoding="utf-8",
        )
    finally:
        os.remove(cfg)

    body, _, code = r.stdout.rpartition("\n")
    result = {"repository": REPO, "branch": "main", "http": int(code or 0), "body": body}
    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"http={code} -> {OUT}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

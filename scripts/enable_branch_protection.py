#!/usr/bin/env python3
"""尝试为 main 开启分支保护并回读验证；结果写入 docs/testing/branch_protection.json。"""
import json
import os
import subprocess
import tempfile
from pathlib import Path

REPO = "sunyuan-rgb/aitv"
OUT = Path("docs/testing/branch_protection.json")
API = f"https://api.github.com/repos/{REPO}/branches/main/protection"

PAYLOAD = {
    "required_status_checks": {
        "strict": True,
        "contexts": ["质量门禁（格式/静态检查/测试/覆盖率）"],
    },
    "enforce_admins": False,
    "required_pull_request_reviews": {"required_approving_review_count": 0},
    "restrictions": None,
    "allow_force_pushes": False,
    "allow_deletions": False,
}


def credential() -> str:
    p = subprocess.run(
        ["git", "credential", "fill"],
        input="protocol=https\nhost=github.com\n\n",
        capture_output=True, text=True,
    )
    tokens = [ln[len("password="):] for ln in p.stdout.splitlines() if ln.startswith("password=")]
    return tokens[0] if tokens else ""


def curl(cfg: str, method: str, url: str, payload: dict | None = None):
    args = ["curl", "-s", "-K", cfg, "-X", method, url, "-w", "\n%{http_code}"]
    tmp = None
    if payload is not None:
        fd, tmp = tempfile.mkstemp(suffix=".json")
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump(payload, f, ensure_ascii=False)
        args += ["-H", "Content-Type: application/json", "--data-binary", f"@{tmp}"]
    try:
        r = subprocess.run(args, capture_output=True, text=True, encoding="utf-8")
    finally:
        if tmp:
            os.remove(tmp)
    body, _, code = r.stdout.rpartition("\n")
    return int(code or 0), body


def main() -> int:
    tk = credential()
    if not tk:
        print("未能获取 GitHub 凭据")
        return 1
    fd, cfg = tempfile.mkstemp(suffix=".curlrc")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            f.write(f'header = "Authorization: token {tk}"\n')
            f.write('header = "User-Agent: aitv-dev"\n')
            f.write('header = "Accept: application/vnd.github+json"\n')
        put_code, put_body = curl(cfg, "PUT", API, PAYLOAD)
        get_code, get_body = curl(cfg, "GET", API)
    finally:
        os.remove(cfg)

    result = {
        "repository": REPO,
        "branch": "main",
        "enable_request": {"http": put_code, "body": put_body},
        "verify_read": {"http": get_code, "body": get_body},
    }
    OUT.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"PUT http={put_code}, GET http={get_code} -> {OUT}")
    return 0 if put_code in (200, 201) and get_code == 200 else 1


if __name__ == "__main__":
    raise SystemExit(main())

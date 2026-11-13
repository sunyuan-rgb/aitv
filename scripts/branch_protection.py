#!/usr/bin/env python3
"""读取/尝试配置 main 分支保护，并保存真实结果到 docs/testing/branch_protection.json。"""
import json
import subprocess
import sys
import urllib.error
import urllib.request
from pathlib import Path

REPO = "sunyuan-rgb/aitv"
OUT = Path("docs/testing/branch_protection.json")


def token() -> str:
    p = subprocess.run(
        ["git", "credential", "fill"],
        input="protocol=https\nhost=github.com\n\n",
        capture_output=True, text=True,
    )
    for line in p.stdout.splitlines():
        if line.startswith("password="):
            return line[len("password="):]
    sys.exit("未能获取 GitHub 凭据")


def call(method, path, tk, payload=None):
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(
        f"https://api.github.com{path}", data=data, method=method,
        headers={
            "Authorization": f"token {tk}",
            "User-Agent": "aitv-dev",
            "Accept": "application/vnd.github+json",
            "Content-Type": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return r.getcode(), json.loads(r.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        body = e.read().decode()
        try:
            return e.code, json.loads(body or "{}")
        except json.JSONDecodeError:
            return e.code, {"raw": body}


def main() -> int:
    tk = token()
    result = {"repository": REPO, "branch": "main", "actions": []}

    code, data = call("GET", f"/repos/{REPO}/branches/main/protection", tk)
    result["actions"].append({"action": "read_protection", "http": code, "body": data})

    if code == 404:
        # 未开启：尝试开启基础保护（需要管理员权限）
        payload = {
            "required_status_checks": {"strict": True, "contexts": ["质量门禁（格式/静态检查/测试/覆盖率）"]},
            "enforce_admins": False,
            "required_pull_request_reviews": {"required_approving_review_count": 0},
            "restrictions": None,
            "allow_force_pushes": False,
            "allow_deletions": False,
        }
        code2, data2 = call("PUT", f"/repos/{REPO}/branches/main/protection", tk, payload)
        result["actions"].append({"action": "enable_protection", "http": code2, "body": data2})
        if code2 in (200, 201):
            code3, data3 = call("GET", f"/repos/{REPO}/branches/main/protection", tk)
            result["actions"].append({"action": "read_protection_after_enable", "http": code3, "body": data3})

    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"written {OUT}，最后 HTTP: {result['actions'][-1]['http']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

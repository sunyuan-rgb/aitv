#!/usr/bin/env python3
"""PR 全流程（curl 实现）：创建 PR -> 轮询检查 -> merge commit 合并。

用法: python scripts/pr_flow.py [--merge]
"""
import json
import os
import subprocess
import sys
import tempfile
import time
from pathlib import Path

REPO = "sunyuan-rgb/aitv"
HEAD = "dev/04"
BASE = "main"
TITLE = "[feat] 题目04 短剧内容上架、付费观看与下架处理"
BODY = (
    "题目04 完整实现与验收材料：Gin + SQLite 业务闭环；两个自编 Skill；"
    "质量门禁（gofmt/go vet/go test，整体与核心包覆盖率 ≥85%）；"
    "CI 流水线、Dockerfile、部署与业务冒烟；验收报告 docs/acceptance/report.md"
)


def credential() -> str:
    p = subprocess.run(
        ["git", "credential", "fill"],
        input="protocol=https\nhost=github.com\n\n",
        capture_output=True, text=True,
    )
    tokens = [ln[len("password="):] for ln in p.stdout.splitlines() if ln.startswith("password=")]
    if not tokens:
        sys.exit("未能获取 GitHub 凭据")
    return tokens[0]


class Curl:
    def __init__(self, token: str):
        fd, self.cfg = tempfile.mkstemp(suffix=".curlrc")
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            f.write(f'header = "Authorization: token {token}"\n')
            f.write('header = "User-Agent: aitv-dev"\n')
            f.write('header = "Accept: application/vnd.github+json"\n')

    def close(self):
        os.remove(self.cfg)

    def call(self, method: str, path: str, payload=None):
        args = ["curl", "-s", "-K", self.cfg, "-X", method, f"https://api.github.com{path}",
                "-w", "\n%{http_code}"]
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
        try:
            return int(code or 0), json.loads(body or "{}")
        except json.JSONDecodeError:
            return int(code or 0), {"raw": body}


def main() -> int:
    c = Curl(credential())
    try:
        code, pr = c.call("POST", f"/repos/{REPO}/pulls",
                          {"title": TITLE, "head": HEAD, "base": BASE, "body": BODY})
        if code not in (200, 201):
            code, prs = c.call("GET", f"/repos/{REPO}/pulls?head={REPO.split('/')[0]}:{HEAD}&state=open")
            if code != 200 or not prs:
                print(f"创建 PR 失败: {code} {pr.get('message')}")
                return 1
            pr = prs[0]
        print(f"PR #{pr['number']} {pr['html_url']} state={pr['state']}")

        if "--merge" not in sys.argv:
            return 0

        sha = pr["head"]["sha"]
        merged = False
        for _ in range(40):
            code, checks = c.call("GET", f"/repos/{REPO}/commits/{sha}/check-runs?per_page=50")
            runs = checks.get("check_runs", []) if code == 200 else []
            if runs and all(r["status"] == "completed" for r in runs):
                bad = [(r["name"], r["conclusion"]) for r in runs
                       if r["conclusion"] not in ("success", "skipped", "neutral")]
                if bad:
                    print("检查未通过:", bad)
                    return 1
                print("检查通过:", [r["name"] for r in runs])
                code, res = c.call("PUT", f"/repos/{REPO}/pulls/{pr['number']}/merge",
                                   {"merge_method": "merge",
                                    "commit_title": f"Merge pull request #{pr['number']} from {HEAD}"})
                if code == 200 and res.get("merged"):
                    print(f"已合并: {res['sha']}")
                    merged = True
                    break
                print(f"合并失败: {code} {res.get('message')}")
                return 1
            time.sleep(15)
        if not merged:
            print("等待检查超时，未合并")
            return 1
        return 0
    finally:
        c.close()


if __name__ == "__main__":
    raise SystemExit(main())

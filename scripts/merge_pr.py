#!/usr/bin/env python3
"""等待 PR 检查通过后以 merge commit 合并（凭据取自 git credential fill，不输出）。"""
import json
import subprocess
import sys
import time
import urllib.error
import urllib.request

REPO = "sunyuan-rgb/aitv"
HEAD = "dev/04"


def token() -> str:
    p = subprocess.run(
        ["git", "credential", "fill"],
        input="protocol=https\nhost=github.com\n\n",
        capture_output=True, text=True,
    )
    for ln in p.stdout.splitlines():
        if ln.startswith("password="):
            return ln[len("password="):]
    sys.exit("未能获取 GitHub 凭据")


def api(method: str, path: str, tk: str, payload=None):
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
        return e.code, json.loads(e.read().decode() or "{}")


def main() -> int:
    tk = token()
    code, prs = api("GET", f"/repos/{REPO}/pulls?head={REPO.split('/')[0]}:{HEAD}&state=open", tk)
    if code != 200 or not prs:
        print("没有打开中的 PR")
        return 1
    pr = prs[0]
    print(f"PR #{pr['number']} {pr['html_url']}")
    sha = pr["head"]["sha"]

    for _ in range(40):  # 最多约 10 分钟
        c, checks = api("GET", f"/repos/{REPO}/commits/{sha}/check-runs?per_page=50", tk)
        runs = checks.get("check_runs", []) if c == 200 else []
        if runs and all(r["status"] == "completed" for r in runs):
            bad = [r["name"] for r in runs if r["conclusion"] not in ("success", "skipped", "neutral")]
            if bad:
                print("检查未通过:", bad)
                return 1
            print("检查通过:", [r["name"] for r in runs])
            break
        time.sleep(15)

    code, res = api("PUT", f"/repos/{REPO}/pulls/{pr['number']}/merge", tk,
                    {"merge_method": "merge",
                     "commit_title": f"Merge pull request #{pr['number']} from {HEAD}"})
    if code == 200 and res.get("merged"):
        print(f"已合并: {res['sha']}")
        return 0
    print(f"合并失败: {code} {res.get('message')}")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())

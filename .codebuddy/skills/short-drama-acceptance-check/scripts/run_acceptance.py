#!/usr/bin/env python3
"""题目04 验收脚本：质量门禁 + 覆盖率门禁 + 部署冒烟。

用法:
    python run_acceptance.py [--base-url http://localhost:8080]
退出码: 0 = 验收通过; 1 = 验收不通过。
"""
import argparse
import json
import re
import subprocess
import sys
import urllib.parse
import urllib.request
import urllib.error

CORE_PKGS = ["content", "billing", "sales", "refund"]
GATE = 85.0


def run(cmd, cwd=None):
    p = subprocess.run(cmd, shell=True, capture_output=True, text=True, cwd=cwd)
    return p.returncode, (p.stdout or "") + (p.stderr or "")


def check_gofmt():
    code, out = run("gofmt -l .")
    files = [ln for ln in out.splitlines() if ln.strip()]
    return (len(files) == 0, out or "(无未格式化文件)")


def check_vet():
    code, out = run("go vet ./...")
    return (code == 0, out or "(无告警)")


def check_tests():
    code, out = run("go test ./...")
    passed = code == 0
    m = re.findall(r"(ok|FAIL)\s+aitv/(\S+)", out)
    detail = "; ".join(f"{pkg}: {st}" for st, pkg in m)
    return (passed, detail or out[-500:])


def check_coverage():
    code, out = run("go test ./... -coverprofile=coverage.out.tmp -covermode=atomic")
    # 各包覆盖率来自 go test 输出（包级语句覆盖率）
    pkg_cov = {}
    for ln in out.splitlines():
        m = re.match(r"ok\s+aitv/internal/(\w+)\s.*coverage:\s*([\d.]+)%", ln)
        if m:
            pkg_cov[m.group(1)] = float(m.group(2))
    # 整体覆盖率来自 cover -func
    code2, func_out = run("go tool cover -func coverage.out.tmp")
    total = None
    for ln in func_out.splitlines():
        if "total:" in ln:
            m = re.search(r"([\d.]+)%\s*$", ln)
            if m:
                total = float(m.group(1))
    run("del coverage.out.tmp" if sys.platform == "win32" else "rm -f coverage.out.tmp")
    lines = [f"{k} {v:.1f}%" for k, v in sorted(pkg_cov.items())]
    core_ok = all(pkg_cov.get(p, 0) >= GATE for p in CORE_PKGS)
    total_ok = total is not None and total >= GATE
    detail = " ".join(lines) + (f" | 整体 {total:.1f}%" if total is not None else " | 整体不可获取")
    return (core_ok and total_ok, detail)


def smoke(base_url):
    import http.cookiejar
    results = []
    cj = http.cookiejar.CookieJar()
    op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))
    base = base_url.rstrip("/")

    def req(path, data=None, form=False):
        url = base + path
        headers = {}
        body = None
        if data is not None:
            body = data
            headers["Content-Type"] = (
                "application/x-www-form-urlencoded" if form else "application/json"
            )
        r = urllib.request.Request(url, data=body, headers=headers)
        try:
            resp = op.open(r, timeout=10)
            return resp.getcode(), resp.read().decode()
        except urllib.error.HTTPError as e:
            return e.code, e.read().decode()
        except Exception as e:  # 连接失败等
            return 0, f"连接失败: {e}"

    # 健康检查：登录页可达
    sc, raw = req("/login")
    results.append(("登录页可达", sc == 200, f"GET /login -> {sc}"))

    # u1 表单登录（CookieJar 自动维护会话）
    form = urllib.parse.urlencode({"username": "u1", "password": "pass123"}).encode()
    try:
        sc, _ = req("/login", form, form=True)
        logged_in = any(c.name == "aitv_session" for c in cj)
    except Exception as e:  # noqa
        results.append(("登录 u1", False, str(e)))
        return results
    results.append(("登录 u1", logged_in, f"POST /login -> {sc}"))

    # 会话内接口可用性（业务数据完整性由 deploy/smoke.sh 与一致性 Skill 负责）
    sc, raw = req("/api/episodes")
    results.append(("剧集目录接口", sc == 200 and "episodes" in raw, raw[:120]))
    sc, raw = req("/api/me")
    results.append(("我的记录接口", sc == 200 and "Balance" in raw, raw[:120]))
    # 未登录访问受保护接口应被重定向到登录页（urllib 自动跟随重定向）
    anon = urllib.request.build_opener()
    try:
        abody = anon.open(base + "/api/episodes", timeout=10).read().decode()
        results.append(("未登录拦截", "登录" in abody and "episodes" not in abody, "匿名请求被重定向到登录页"))
    except urllib.error.HTTPError as e:
        results.append(("未登录拦截", e.code in (401, 403), f"匿名 /api/episodes -> {e.code}"))
    return results


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--base-url", default=None)
    args = ap.parse_args()

    all_pass = True
    print("== 质量门禁 ==")

    ok, detail = check_gofmt()
    all_pass &= ok
    print(f"gofmt {'PASS' if ok else 'FAIL'}: {detail}")

    ok, detail = check_vet()
    all_pass &= ok
    print(f"go vet {'PASS' if ok else 'FAIL'}: {detail}")

    ok, detail = check_tests()
    all_pass &= ok
    print(f"go test {'PASS' if ok else 'FAIL'}: {detail}")

    ok, detail = check_coverage()
    all_pass &= ok
    print(f"覆盖率门禁(≥{GATE}%，核心包 {'/'.join(CORE_PKGS)}) {'PASS' if ok else 'FAIL'}: {detail}")

    if args.base_url:
        print("== 部署冒烟 ==")
        for name, ok, detail in smoke(args.base_url):
            all_pass &= ok
            print(f"{name} {'PASS' if ok else 'FAIL'}: {detail}")
    else:
        print("== 部署冒烟 == SKIP(未提供 --base-url)，须人工补充部署证据")

    print(f"\n结论: {'验收通过' if all_pass else '验收不通过'}")
    return 0 if all_pass else 1


if __name__ == "__main__":
    sys.exit(main())

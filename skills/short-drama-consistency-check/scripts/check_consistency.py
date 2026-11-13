#!/usr/bin/env python3
"""题目04 业务一致性核查脚本：对 SQLite 执行 checks.sql 并输出 PASS/FAIL 证据。

用法:
    python check_consistency.py --db data/aitv.db
退出码: 0 = 全部通过; 1 = 存在 FAIL; 2 = 执行出错(含 SKIP 项)。
"""
import argparse
import re
import sqlite3
import sys
from pathlib import Path

CHECK_RE = re.compile(r"^--\s*CHECK\s+(\d+):\s*(.+)$", re.M)


def parse_checks(sql_text: str):
    """把 checks.sql 拆成 [(编号, 说明, 主核查SQL, 证据SQL列表)]。"""
    lines = sql_text.splitlines()
    checks = []
    cur = None
    for line in lines:
        m = CHECK_RE.match(line)
        if m:
            cur = {"id": int(m.group(1)), "desc": m.group(2).strip(), "sqls": []}
            checks.append(cur)
            continue
        if cur is not None and line.strip() and not line.strip().startswith("--"):
            cur["sqls"].append(line)
    # 相邻非空 SQL 合并：第一条为主核查，其余为证据
    out = []
    for c in checks:
        buf, stmts = [], []
        for ln in c["sqls"]:
            buf.append(ln)
            if ";" in ln:
                stmts.append(" ".join(buf).strip().rstrip(";"))
                buf = []
        if buf:
            stmts.append(" ".join(buf).strip())
        c["main"] = stmts[0] if stmts else None
        c["evidence"] = stmts[1:] if stmts else []
        out.append(c)
    return out


def fmt_rows(rows, cols):
    if not rows:
        return "  (无违规/无数据)"
    return "\n".join("  " + " | ".join(str(v) for v in row) for row in rows)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default="data/aitv.db", help="SQLite 数据库路径")
    args = ap.parse_args()

    sql_path = Path(__file__).parent / "checks.sql"
    checks = parse_checks(sql_path.read_text(encoding="utf-8"))

    con = sqlite3.connect(f"file:{args.db}?mode=ro", uri=True)  # 只读核查
    con.row_factory = sqlite3.Row
    fails, skips, passes = [], [], []

    print(f"=== 短剧业务一致性核查 (db={args.db}) ===")
    for c in checks:
        if not c["main"]:
            skips.append(c["id"])
            print(f"[CHECK {c['id']}] {c['desc']} ... SKIP(无核查SQL)")
            continue
        try:
            rows = con.execute(c["main"]).fetchall()
            viol = rows[0][0] if rows else 0
            status = "PASS" if viol == 0 else "FAIL"
            print(f"[CHECK {c['id']}] {c['desc']} ... {status} (违规行数={viol})")
            if viol:
                fails.append(c["id"])
                print(f"  主核查SQL: {c['main']}")
            for ev in c["evidence"]:
                erows = con.execute(ev).fetchall()
                if erows:
                    cols = erows[0].keys()
                    print(f"  证据: {fmt_rows([tuple(r) for r in erows], cols)}")
            if viol == 0:
                passes.append(c["id"])
        except sqlite3.Error as e:
            skips.append(c["id"])
            print(f"[CHECK {c['id']}] {c['desc']} ... SKIP(SQL错误: {e})")

    con.close()
    total = len(checks)
    print(f"\n结论: {len(fails)} FAIL / {len(passes)} PASS / {len(skips)} SKIP，共 {total} 项")
    if fails:
        print("业务一致性未通过，FAIL 项编号:", fails)
        print("建议: 定位 internal/billing、internal/refund、internal/sales 中对应逻辑，修复后重跑本脚本复测。")
        return 1
    if skips:
        print("存在 SKIP 项，核查覆盖不完整，须人工补充确认。")
        return 2
    print("业务一致性核查全部通过。")
    return 0


if __name__ == "__main__":
    sys.exit(main())

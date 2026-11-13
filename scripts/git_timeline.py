#!/usr/bin/env python3
"""导出 git 提交时间线（用于 docs/events.jsonl 与文档索引）。"""
import subprocess

out = subprocess.run(
    ["git", "log", "--pretty=format:%h|%ad|%s", "--date=format:%Y-%m-%dT%H:%M:%S", "-20"],
    capture_output=True, text=True, encoding="utf-8",
)
print(out.stdout)

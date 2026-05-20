#!/usr/bin/env python3
"""Show zsh history with human-readable timestamps.

Usage:
  scripts/history.py          # last 50 entries
  scripts/history.py -n 100   # last N entries
  scripts/history.py -g debug # grep for a pattern
"""

import argparse
import re
import subprocess
import sys
from pathlib import Path


def parse_args():
    p = argparse.ArgumentParser(description="zsh history with timestamps")
    p.add_argument("-n", type=int, default=50, metavar="N", help="number of entries to show (default: 50)")
    p.add_argument("-g", "--grep", metavar="PATTERN", help="filter entries matching pattern (case-insensitive)")
    p.add_argument("-f", "--file", default=Path.home() / ".zsh_history", metavar="FILE", help="history file path")
    return p.parse_args()


def read_entries(path, n, pattern):
    text = Path(path).read_bytes().decode("utf-8", errors="replace")
    entries = []
    for line in text.splitlines():
        m = re.match(r": (\d+):\d+;(.*)", line)
        if not m:
            continue
        ts, cmd = int(m.group(1)), m.group(2)
        if pattern and not re.search(pattern, cmd, re.IGNORECASE):
            continue
        entries.append((ts, cmd))
    return entries[-n:]


def format_ts(ts):
    return subprocess.check_output(["date", "-r", str(ts), "+%Y-%m-%d %H:%M:%S"]).decode().strip()


def main():
    args = parse_args()
    try:
        entries = read_entries(args.file, args.n, args.grep)
    except FileNotFoundError:
        sys.exit(f"history file not found: {args.file}")

    for ts, cmd in entries:
        print(f"{format_ts(ts)}  {cmd}")


if __name__ == "__main__":
    main()

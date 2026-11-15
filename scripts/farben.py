#!/usr/bin/env python3
"""No colour literal outside the shared look (gestaltung ADR 0001, rule 4).

Pages take their colours from mdopp/gestaltung, embedded at build time, as
var(--…). This fails on a hex colour, rgb()/rgba() or hsl()/hsla() in any
tracked page source (.go, .html, .css, .js, .svg, .json, …), except:

- a synced copy of the shared look (a file named gestaltung.css),
- tests (they may name a colour to prove it is gone),
- what scripts/farben-erlaubt.txt names: content that is not chrome, such as
  a colour the user chose or a chart's data series. One line per file:
  `<path> <literal> [<literal> …]  # why`. An entry that no longer matches
  fails too, so the list stays short and true.

A colour the program needs and the shared look lacks is added to
gestaltung first, as a token. Usage: scripts/farben.py [repo dir]
"""
import os
import re
import subprocess
import sys

EXT = {".go", ".html", ".htm", ".tmpl", ".gohtml", ".css", ".js", ".mjs", ".ts",
       ".svg", ".json", ".webmanifest", ".xml"}
HEX = re.compile(r"(?<![\w&/#-])#(?:[0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{3,4})(?![\w-])")
FUNC = re.compile(r"(?<![\w.$])(?:rgba?|hsla?)\s*\([^)]+\)", re.I)
# A Go or JS comment is prose ("issue #376"), not a colour.
COMMENT = re.compile(r"(?:^\s*//.*$|\s//\s.*$)")
ALLOW = "scripts/farben-erlaubt.txt"


def is_test(path):
    name = os.path.basename(path)
    return name.endswith("_test.go") or name.startswith("test_") or name.endswith("_test.py") \
        or "/testdata/" in "/" + path


def literals(text, comments):
    for n, line in enumerate(text.splitlines(), 1):
        if comments:
            line = COMMENT.sub("", line)
        for m in HEX.finditer(line):
            yield n, m.group(0)
        for m in FUNC.finditer(line):
            yield n, re.sub(r"\s+", "", m.group(0)).lower()


def allowlist(root):
    allowed = {}
    try:
        with open(os.path.join(root, ALLOW), encoding="utf-8") as f:
            for line in f:
                # a hex literal starts with '#': split on ' # ' (the reason), not on '#'
                words = line.split(" # ", 1)[0].split() if not line.lstrip().startswith("#") else []
                if words:
                    allowed.setdefault(words[0], set()).update(w.lower() for w in words[1:])
    except FileNotFoundError:
        pass
    return allowed


def files(root):
    out = subprocess.run(["git", "-C", root, "ls-files", "-z"], capture_output=True, check=True).stdout
    return sorted(p for p in out.decode().split("\0") if p)


def check(root):
    allowed = allowlist(root)
    used = {p: set() for p in allowed}
    bad = []
    for path in files(root):
        if os.path.splitext(path)[1].lower() not in EXT or os.path.basename(path) == "gestaltung.css" or is_test(path):
            continue
        try:
            with open(os.path.join(root, path), encoding="utf-8") as f:
                text = f.read()
        except (UnicodeDecodeError, FileNotFoundError):
            continue
        comments = path.endswith((".go", ".js", ".mjs", ".ts"))
        for n, lit in literals(text, comments):
            if lit.lower() in allowed.get(path, ()):
                used[path].add(lit.lower())
            else:
                bad.append(f"{path}:{n}: {lit}")
    stale = [f"{ALLOW}: {p} {lit} matches nothing any more: remove it"
             for p, lits in allowed.items() for lit in sorted(lits - used.get(p, set()))]
    return bad, stale


def main():
    root = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(__file__), "..")
    bad, stale = check(root)
    for b in bad:
        print(b)
    for s in stale:
        print(s)
    if bad:
        print(f"{len(bad)} colour literal(s) outside the shared look: use a var(--…) token from "
              f"mdopp/gestaltung, add the token there first, or name user content in {ALLOW} with its reason")
    if bad or stale:
        return 1
    print("colours: only tokens of the shared look")
    return 0


if __name__ == "__main__":
    sys.exit(main())

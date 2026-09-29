#!/usr/bin/env python3
"""Turn routines in the repository layout (routines/<id>/routine.json,
routine.js, tests.json) into gallery entries (gallery/routines/<id>.json).

Usage: to-gallery.py FOLDER [--rename id=New name ...]

Only the name and description may be changed on the way; code, manifest
and tests are copied as they are, so what was proved is what is published.
"""
import json
import os
import sys


def main():
    folder = sys.argv[1]
    renames = {}
    for arg in sys.argv[2:]:
        if "=" in arg:
            k, v = arg.split("=", 1)
            renames[k] = v
    out_dir = os.path.join(os.path.dirname(__file__), "..", "gallery", "routines")
    for rid in sorted(os.listdir(os.path.join(folder, "routines"))):
        base = os.path.join(folder, "routines", rid)
        if not os.path.isdir(base):
            continue
        r = json.load(open(os.path.join(base, "routine.json"), encoding="utf-8"))
        r["code"] = open(os.path.join(base, "routine.js"), encoding="utf-8").read()
        tests_path = os.path.join(base, "tests.json")
        if os.path.exists(tests_path):
            t = json.load(open(tests_path, encoding="utf-8"))
            r["tests"] = t["tests"] if isinstance(t, dict) else t
        if rid in renames:
            r["name"] = renames[rid]
        entry = {k: r[k] for k in ("name", "description", "manifest", "code", "tests") if k in r}
        path = os.path.join(out_dir, rid + ".json")
        if os.path.exists(path):
            print(f"skip {rid}: already in the gallery", file=sys.stderr)
            continue
        with open(path, "w", encoding="utf-8") as f:
            json.dump(entry, f, ensure_ascii=False, indent=1)
            f.write("\n")
        print(f"{rid}: {entry['name']}")


if __name__ == "__main__":
    main()

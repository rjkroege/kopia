#!/usr/bin/env python3
"""Summarize a Cobertura coverage.xml as Markdown (fork only).

GitHub Code Quality, which upstream uploads coverage to, isn't available
for a personal-account repository, so the fork reports coverage in the job
summary and in a PR comment instead.

Usage: coverage-summary.py coverage.xml [module-prefix]
"""

import sys
import xml.etree.ElementTree as ET

MARKER = "<!-- fork-coverage-summary -->"


def pct(covered, total):
    return 100.0 * covered / total if total else 0.0


def main():
    path = sys.argv[1]
    prefix = sys.argv[2] if len(sys.argv) > 2 else "github.com/kopia/kopia/"

    packages = {}
    for pkg in ET.parse(path).getroot().iter("package"):
        covered = total = 0
        # Count class-level lines only; method-level lines repeat them.
        for cls in pkg.iter("class"):
            lines = cls.find("lines")
            if lines is None:
                continue
            for line in lines.iter("line"):
                total += 1
                if int(line.get("hits", "0")) > 0:
                    covered += 1
        name = pkg.get("name", "")
        name = name[len(prefix):] if name.startswith(prefix) else name
        c, t = packages.get(name or ".", (0, 0))
        packages[name or "."] = (c + covered, t + total)

    covered = sum(c for c, _ in packages.values())
    total = sum(t for _, t in packages.values())

    out = [
        MARKER,
        "### Code coverage",
        "",
        f"**{pct(covered, total):.1f}%** of lines covered ({covered:,} / {total:,}) in {len(packages)} packages.",
        "",
        "<details><summary>Per package</summary>",
        "",
        "| Package | Coverage | Lines |",
        "|---|---:|---:|",
    ]
    for name in sorted(packages):
        c, t = packages[name]
        out.append(f"| `{name}` | {pct(c, t):.1f}% | {c:,} / {t:,} |")
    out += ["", "</details>", ""]

    print("\n".join(out))


if __name__ == "__main__":
    main()

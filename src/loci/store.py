"""The country list: a plain text file, one entry per line, "#" for comments."""

from __future__ import annotations

from pathlib import Path

DEFAULT_PATH = Path("countries.txt")


def load(path: Path = DEFAULT_PATH) -> list[str]:
    if not path.exists():
        return []
    entries = []
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.split("#", 1)[0].strip()
        if line and line not in entries:
            entries.append(line)
    return entries


def save(entries: list[str], path: Path = DEFAULT_PATH) -> None:
    body = "".join(f"{e}\n" for e in entries)
    path.write_text(
        "# Countries tracked by loci. One per line: ISO code or name, optional -SUBDIVISION.\n" + body,
        encoding="utf-8",
    )

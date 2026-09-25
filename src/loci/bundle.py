"""Write the holiday tables out as JSON, for software rather than for a calendar app.

An .ics file is the right answer for a person who wants holidays to appear in
Apple Calendar or Outlook. It is the wrong answer for a program that needs to ask
"is 2027-07-03 a holiday in the US Virgin Islands?" while drawing a page: the
question is a lookup, and a feed is a stream.

So this writes the same data in the shape a lookup wants — one small file per
country, plus an index naming them all. A consumer ships the lot, or fetches one
country when somebody picks it. Five years of a single country is a few kilobytes;
every country at once is under a megabyte.

The files are generated, not authored. Regenerating them is the way to add a year.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

import holidays
from holidays import registry

from .countries import Country, _display_name

# Bumped when the shape of the files changes, so a consumer can refuse a bundle
# it does not understand rather than misreading it.
FORMAT_VERSION = 1


@dataclass(frozen=True)
class BundleStats:
    countries: int
    entries: int
    bytes: int
    skipped: list[str]


def _table(code: str, subdiv: str | None, years: list[int]) -> dict[str, list[str]]:
    """{"2027-07-03": ["Emancipation Day"]} for one country.

    Names are kept as a list because several holidays can fall on one date, and
    collapsing them would quietly drop one. The library joins them with "; ",
    which is a formatting decision this file should not inherit.
    """
    table = holidays.country_holidays(code, subdiv=subdiv, years=years)
    out: dict[str, list[str]] = {}
    for day, names in sorted(table.items()):
        out[day.isoformat()] = [n.strip() for n in names.split(";") if n.strip()]
    return out


def all_countries() -> list[Country]:
    """Every country the holidays library supports, without subdivisions."""
    seen: dict[str, str] = {}
    for class_name, alpha2, *_ in registry.COUNTRIES.values():
        seen.setdefault(alpha2, _display_name(class_name))
    return sorted(
        (Country(code=code, name=name) for code, name in seen.items()),
        key=lambda c: c.name,
    )


def write(
    out_dir: Path,
    years: Iterable[int],
    only: Iterable[Country] | None = None,
) -> BundleStats:
    """Write index.json and one file per country into out_dir.

    ``only`` limits the bundle to the countries given; without it every supported
    country is written, which is what a consumer offering a country picker wants.
    """
    years = list(years)
    targets = list(only) if only is not None else all_countries()

    out_dir.mkdir(parents=True, exist_ok=True)
    days_dir = out_dir / "countries"
    days_dir.mkdir(exist_ok=True)

    index = []
    entries = 0
    total_bytes = 0
    skipped: list[str] = []

    for country in targets:
        try:
            table = _table(country.code, country.subdivision, years)
        except Exception as exc:  # a country the library lists but cannot build
            skipped.append(f"{country.key}: {exc}")
            continue

        body = json.dumps(
            {"country": country.key, "name": country.label, "years": years, "holidays": table},
            separators=(",", ":"),
            ensure_ascii=False,
        )
        path = days_dir / f"{country.key}.json"
        path.write_text(body, encoding="utf-8")

        entries += sum(len(v) for v in table.values())
        total_bytes += len(body.encode("utf-8"))
        index.append({"code": country.key, "name": country.label, "count": len(table)})

    index.sort(key=lambda row: row["name"])
    (out_dir / "index.json").write_text(
        json.dumps(
            {"format": FORMAT_VERSION, "years": years, "countries": index},
            separators=(",", ":"),
            ensure_ascii=False,
        ),
        encoding="utf-8",
    )

    return BundleStats(
        countries=len(index), entries=entries, bytes=total_bytes, skipped=skipped
    )

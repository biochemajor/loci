"""Export holiday data for the web page to web/holidays.json.

    python web/build_data.py 2026 2028

The page can't run the Python `holidays` library, so this precomputes every
supported country (and its subdivisions, stored as differences from the
national list) for a range of years.
"""

from __future__ import annotations

import json
import sys
import warnings
from datetime import date
from pathlib import Path

import holidays

from loci import countries, ics

OUT = Path(__file__).with_name("holidays.json")


def pairs(found):
    return [[h.day.isoformat(), h.name] for h in found]


def build(years: list[int]) -> dict:
    data = {}
    for c in countries.supported():
        cls = getattr(holidays, c.code)
        national = pairs(ics.collect([c], years))
        national_set = {tuple(p) for p in national}
        names = {code: name for name, code in getattr(cls, "subdivisions_aliases", {}).items()}
        subs = {}
        for s in cls.subdivisions:
            try:
                regional = pairs(ics.collect([countries.Country(c.code, c.name, s)], years))
            except Exception:  # a few subdivisions fail for some years; skip them
                continue
            regional_set = {tuple(p) for p in regional}
            entry = {"+": [p for p in regional if tuple(p) not in national_set]}
            missing = [p for p in national if tuple(p) not in regional_set]
            if missing:
                entry["-"] = missing
            if s in names:
                entry["n"] = names[s]
            subs[s] = entry
        data[c.code] = {"n": c.name, "h": national, **({"s": subs} if subs else {})}
    return {"years": years, "generated": date.today().isoformat(), "countries": data}


def main() -> None:
    first, last = (int(a) for a in sys.argv[1:3]) if len(sys.argv) >= 3 else (date.today().year, date.today().year + 2)
    warnings.filterwarnings("ignore")
    payload = build(list(range(first, last + 1)))
    OUT.write_text(json.dumps(payload, ensure_ascii=False, separators=(",", ":")), encoding="utf-8")
    print(f"Wrote {len(payload['countries'])} countries, {first}-{last}, to {OUT} ({OUT.stat().st_size // 1024} KB)")


if __name__ == "__main__":
    main()

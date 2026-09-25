"""Command-line interface: loci add | remove | list | countries | preview | generate."""

from __future__ import annotations

import argparse
import sys
from datetime import date
from pathlib import Path

from . import __version__, bundle, countries, ics, store


def _parse_years(spec: str) -> list[int]:
    """"2027" -> [2027]; "2026-2028" -> [2026, 2027, 2028]."""
    try:
        if "-" in spec:
            lo, hi = (int(p) for p in spec.split("-", 1))
        else:
            lo = hi = int(spec)
    except ValueError:
        raise argparse.ArgumentTypeError(f"invalid year or range: {spec!r}") from None
    if lo > hi:
        raise argparse.ArgumentTypeError(f"range is backwards: {spec!r}")
    return list(range(lo, hi + 1))


def _tracked(path: Path) -> list[countries.Country]:
    tracked = []
    for entry in store.load(path):
        try:
            tracked.append(countries.resolve(entry))
        except countries.UnknownCountryError as e:
            print(f"warning: skipping {e}", file=sys.stderr)
    return tracked


def cmd_add(args) -> int:
    entries = store.load(args.file)
    status = 0
    for text in args.countries:
        try:
            c = countries.resolve(text)
        except countries.UnknownCountryError as e:
            print(f"error: {e}", file=sys.stderr)
            status = 1
            continue
        if c.key in entries:
            print(f"  already tracking {c.flag} {c.label} [{c.key}]")
        else:
            entries.append(c.key)
            print(f"+ added {c.flag} {c.label} [{c.key}]")
    store.save(entries, args.file)
    return status


def cmd_remove(args) -> int:
    entries = store.load(args.file)
    status = 0
    for text in args.countries:
        try:
            key = countries.resolve(text).key
        except countries.UnknownCountryError:
            key = text.strip().upper()
        if key in entries:
            entries.remove(key)
            print(f"- removed {key}")
        else:
            print(f"error: {text!r} is not in the list", file=sys.stderr)
            status = 1
    store.save(entries, args.file)
    return status


def cmd_list(args) -> int:
    tracked = _tracked(args.file)
    if not tracked:
        print("No countries yet. Add some with: loci add japan US-CA DE")
        return 0
    for c in tracked:
        print(f"{c.flag}  {c.key:<8} {c.label}")
    return 0


def cmd_countries(args) -> int:
    needle = (args.search or "").lower()
    for c in countries.supported():
        if needle in c.name.lower() or needle == c.code.lower():
            print(f"{c.flag}  {c.code}  {c.name}")
    return 0


def _holidays(args):
    tracked = _tracked(args.file)
    if not tracked:
        print("error: no countries in the list. Add some with: loci add japan", file=sys.stderr)
        return None
    return ics.collect(tracked, args.years, language=args.language)


def cmd_preview(args) -> int:
    found = _holidays(args)
    if found is None:
        return 1
    for h in found:
        print(f"{h.day:%a %Y-%m-%d}  {h.country.flag} {h.country.label}: {h.name}")
    print(f"\n{len(found)} holidays")
    return 0


def cmd_generate(args) -> int:
    found = _holidays(args)
    if found is None:
        return 1
    text = ics.render(found, calendar_name=args.name, flags=not args.no_flags)
    args.output.write_text(text, encoding="utf-8", newline="")
    years = f"{args.years[0]}" if len(args.years) == 1 else f"{args.years[0]}-{args.years[-1]}"
    print(f"Wrote {len(found)} holidays ({years}) to {args.output}")
    print("Import it into Google Calendar, Outlook or Apple Calendar.")
    return 0


def cmd_bundle(args) -> int:
    """Write the JSON bundle a program reads, rather than the .ics a person subscribes to."""
    only = _tracked(args.file) if args.tracked else None
    if args.tracked and not only:
        print("no countries tracked; add some first, or drop --tracked", file=sys.stderr)
        return 1

    stats = bundle.write(args.output, args.years, only)
    for problem in stats.skipped:
        print(f"warning: skipped {problem}", file=sys.stderr)
    print(
        f"{stats.countries} countries, {stats.entries} holidays, "
        f"{stats.bytes / 1024:.0f} KB -> {args.output}/"
    )
    return 0


def build_parser() -> argparse.ArgumentParser:
    this_year = date.today().year
    p = argparse.ArgumentParser(prog="loci", description="Find public holidays for your countries and export an .ics calendar file.")
    p.add_argument("--version", action="version", version=f"loci {__version__}")
    p.add_argument(
        "-f", "--file", type=Path, default=store.DEFAULT_PATH,
        help="country list file (default: countries.txt)",
    )
    sub = p.add_subparsers(dest="command", required=True)

    a = sub.add_parser("add", help="add countries (code or name, e.g. JP, japan, US-CA)")
    a.add_argument("countries", nargs="+")
    a.set_defaults(func=cmd_add)

    r = sub.add_parser("remove", help="remove countries from the list")
    r.add_argument("countries", nargs="+")
    r.set_defaults(func=cmd_remove)

    sub.add_parser("list", help="show tracked countries").set_defaults(func=cmd_list)

    c = sub.add_parser("countries", help="show all supported countries")
    c.add_argument("search", nargs="?", help="filter by name or code")
    c.set_defaults(func=cmd_countries)

    b = sub.add_parser("bundle", help="write holiday tables as JSON for another program to read")
    b.add_argument(
        "-y", "--years", type=_parse_years, default=[this_year, this_year + 1],
        help=f"year or range, e.g. 2027 or 2026-2030 (default: {this_year}-{this_year + 1})",
    )
    b.add_argument("-o", "--output", type=Path, default=Path("holidays-json"), help="directory to write into")
    b.add_argument(
        "--tracked", action="store_true",
        help="only the countries in the list; without this, every supported country",
    )
    b.set_defaults(func=cmd_bundle)

    for name, func, helptext in (
        ("preview", cmd_preview, "print holidays to the terminal"),
        ("generate", cmd_generate, "write an .ics calendar file"),
    ):
        g = sub.add_parser(name, help=helptext)
        g.add_argument(
            "-y", "--years", type=_parse_years, default=[this_year, this_year + 1],
            help=f"year or range, e.g. 2027 or 2026-2028 (default: {this_year}-{this_year + 1})",
        )
        g.add_argument("--language", default="en_US", help="holiday name language if available (default: en_US)")
        g.set_defaults(func=func)
        if name == "generate":
            g.add_argument("-o", "--output", type=Path, default=Path("holidays.ics"))
            g.add_argument("--name", default="International Holidays", help="calendar name")
            g.add_argument("--no-flags", action="store_true", help="omit flag emoji from event titles")
    return p


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())

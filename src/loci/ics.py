"""Collect holidays for countries and render them as an RFC 5545 iCalendar file."""

from __future__ import annotations

import hashlib
from dataclasses import dataclass
from datetime import date, datetime, timedelta, timezone
from typing import Iterable

import holidays

from .countries import Country

PRODID = "-//loci//International Holidays//EN"


@dataclass(frozen=True)
class Holiday:
    day: date
    name: str
    country: Country

    @property
    def uid(self) -> str:
        # Stable across regenerations so calendar apps update events instead of duplicating them.
        digest = hashlib.sha1(f"{self.country.key}|{self.day}|{self.name}".encode()).hexdigest()[:16]
        return f"{self.day:%Y%m%d}-{self.country.key.lower()}-{digest}@loci"


def collect(countries: Iterable[Country], years: Iterable[int], language: str = "en_US") -> list[Holiday]:
    years = list(years)
    result = []
    for country in countries:
        cls = getattr(holidays, country.code)
        lang = language if language in getattr(cls, "supported_languages", ()) else None
        table = holidays.country_holidays(
            country.code, subdiv=country.subdivision, years=years, language=lang
        )
        for day, names in sorted(table.items()):
            # Several holidays can share a date; the library joins them with "; ".
            for name in names.split("; "):
                result.append(Holiday(day, name, country))
    result.sort(key=lambda h: (h.day, h.country.label, h.name))
    return result


def _escape(text: str) -> str:
    return (
        text.replace("\\", "\\\\").replace(";", "\\;").replace(",", "\\,").replace("\n", "\\n")
    )


def _fold(line: str) -> str:
    """Fold a content line to at most 75 octets per RFC 5545 §3.1."""
    raw = line.encode("utf-8")
    if len(raw) <= 75:
        return line
    parts, start, limit = [], 0, 75
    while start < len(raw):
        end = min(start + limit, len(raw))
        while end < len(raw) and (raw[end] & 0xC0) == 0x80:  # don't split a UTF-8 sequence
            end -= 1
        parts.append(raw[start:end].decode("utf-8"))
        start, limit = end, 74  # continuation lines start with a space
    return "\r\n ".join(parts)


def render(
    entries: Iterable[Holiday],
    calendar_name: str = "International Holidays",
    flags: bool = True,
    now: datetime | None = None,
) -> str:
    stamp = (now or datetime.now(timezone.utc)).strftime("%Y%m%dT%H%M%SZ")
    lines = [
        "BEGIN:VCALENDAR",
        "VERSION:2.0",
        f"PRODID:{PRODID}",
        "CALSCALE:GREGORIAN",
        "METHOD:PUBLISH",
        f"X-WR-CALNAME:{_escape(calendar_name)}",
    ]
    for h in entries:
        prefix = f"{h.country.flag} " if flags else ""
        lines += [
            "BEGIN:VEVENT",
            f"UID:{h.uid}",
            f"DTSTAMP:{stamp}",
            f"DTSTART;VALUE=DATE:{h.day:%Y%m%d}",
            f"DTEND;VALUE=DATE:{h.day + timedelta(days=1):%Y%m%d}",
            f"SUMMARY:{_escape(f'{prefix}{h.country.label}: {h.name}')}",
            f"CATEGORIES:{_escape(h.country.name)}",
            f"DESCRIPTION:{_escape(f'Public holiday in {h.country.label}.')}",
            "TRANSP:TRANSPARENT",  # don't mark you as busy
            "END:VEVENT",
        ]
    lines.append("END:VCALENDAR")
    return "\r\n".join(_fold(line) for line in lines) + "\r\n"

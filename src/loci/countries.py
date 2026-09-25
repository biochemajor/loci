"""Resolve user input ("japan", "JP", "JPN", "US-CA") to a supported country."""

from __future__ import annotations

import re
from dataclasses import dataclass
from functools import lru_cache

import holidays
from holidays import registry


@dataclass(frozen=True)
class Country:
    code: str  # ISO 3166-1 alpha-2, e.g. "US"
    name: str  # e.g. "United States"
    subdivision: str | None = None  # e.g. "CA"

    @property
    def key(self) -> str:
        """Canonical form stored in the country list, e.g. "US" or "US-CA"."""
        return f"{self.code}-{self.subdivision}" if self.subdivision else self.code

    @property
    def label(self) -> str:
        return f"{self.name} ({self.subdivision})" if self.subdivision else self.name

    @property
    def flag(self) -> str:
        """Regional-indicator emoji flag for the alpha-2 code."""
        return "".join(chr(0x1F1E6 + ord(c) - ord("A")) for c in self.code)


def _display_name(class_name: str) -> str:
    # "UnitedStates" -> "United States", "AlandIslands" -> "Aland Islands"
    return re.sub(r"(?<=[a-z])(?=[A-Z])", " ", class_name)


def _normalize(text: str) -> str:
    return re.sub(r"[^a-z0-9]", "", text.lower())


@lru_cache(maxsize=1)
def _index() -> tuple[dict[str, str], dict[str, str]]:
    """Return (lookup -> alpha-2 code, alpha-2 code -> display name)."""
    lookup: dict[str, str] = {}
    names: dict[str, str] = {}
    for class_name, alpha2, alpha3, *_ in registry.COUNTRIES.values():
        name = _display_name(class_name)
        names[alpha2] = name
        for alias in (alpha2, alpha3, name, class_name):
            lookup[_normalize(alias)] = alpha2
    return lookup, names


class UnknownCountryError(ValueError):
    pass


def resolve(text: str) -> Country:
    """Turn free-form input into a Country, raising UnknownCountryError if unsupported.

    Accepts alpha-2/alpha-3 codes or English names, optionally with a
    subdivision suffix: "US-CA", "Germany-BY", "GBR-SCT".
    """
    raw = text.strip()
    if not raw:
        raise UnknownCountryError("empty country")
    lookup, names = _index()

    base, subdivision = raw, None
    if _normalize(raw) not in lookup and "-" in raw:
        base, _, subdivision = raw.rpartition("-")
        subdivision = subdivision.strip()

    code = lookup.get(_normalize(base))
    if code is None:
        raise UnknownCountryError(f"unknown or unsupported country: {text!r}")

    if subdivision:
        cls = getattr(holidays, code)
        # Match codes ("BY") and names ("Bavaria") case-insensitively.
        aliases = {_normalize(s): s for s in cls.subdivisions}
        aliases.update({_normalize(k): v for k, v in getattr(cls, "subdivisions_aliases", {}).items()})
        subdivision = aliases.get(_normalize(subdivision), subdivision)
        if subdivision not in cls.subdivisions:
            raise UnknownCountryError(
                f"{names[code]} has no subdivision {subdivision!r}; "
                f"valid: {', '.join(cls.subdivisions) or 'none'}"
            )
    return Country(code=code, name=names[code], subdivision=subdivision)


def supported() -> list[Country]:
    _, names = _index()
    return sorted((Country(code, name) for code, name in names.items()), key=lambda c: c.name)

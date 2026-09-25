from datetime import date, datetime, timezone

import pytest

from loci import countries, ics, store
from loci.cli import main


def test_resolve_codes_names_and_subdivisions():
    assert countries.resolve("jp").key == "JP"
    assert countries.resolve("JPN").key == "JP"
    assert countries.resolve("United States").key == "US"
    assert countries.resolve("us-california").key == "US-CA"
    assert countries.resolve("Germany-BY").label == "Germany (BY)"
    assert countries.resolve("US").flag == "\U0001F1FA\U0001F1F8"


@pytest.mark.parametrize("bad", ["", "narnia", "US-ZZ"])
def test_resolve_rejects_unknown(bad):
    with pytest.raises(countries.UnknownCountryError):
        countries.resolve(bad)


def test_collect_includes_known_holidays():
    found = ics.collect([countries.resolve("JP"), countries.resolve("US")], [2027])
    pairs = {(h.country.code, h.day, h.name) for h in found}
    assert ("JP", date(2027, 1, 1), "New Year's Day") in pairs
    assert ("US", date(2027, 7, 4), "Independence Day") in pairs
    assert found == sorted(found, key=lambda h: (h.day, h.country.label, h.name))


def test_render_is_valid_ics():
    found = ics.collect([countries.resolve("GB-SCT")], [2027])
    text = ics.render(found, now=datetime(2026, 1, 1, tzinfo=timezone.utc))
    lines = text.split("\r\n")
    assert lines[0] == "BEGIN:VCALENDAR" and lines[-2] == "END:VCALENDAR"
    assert text.count("BEGIN:VEVENT") == len(found)
    assert "DTSTART;VALUE=DATE:20271225" in text
    assert all(len(line.encode()) <= 75 for line in lines)
    uids = [line for line in lines if line.startswith("UID:")]
    assert len(uids) == len(set(uids))


def test_uid_is_stable():
    h = ics.Holiday(date(2027, 1, 1), "New Year's Day", countries.resolve("JP"))
    assert h.uid == ics.Holiday(date(2027, 1, 1), "New Year's Day", countries.resolve("japan")).uid


def test_fold_and_escape():
    long = "SUMMARY:" + "日本" * 40
    folded = ics._fold(long)
    assert all(len(p.encode()) <= 75 for p in folded.split("\r\n"))
    assert folded.replace("\r\n ", "") == long
    assert ics._escape("a,b;c\\d\ne") == r"a\,b\;c\\d\ne"


def test_store_roundtrip(tmp_path):
    path = tmp_path / "c.txt"
    store.save(["JP", "US-CA"], path)
    path.write_text(path.read_text() + "JP  # dup\n\n")
    assert store.load(path) == ["JP", "US-CA"]


def test_cli_end_to_end(tmp_path, capsys):
    lst, out = tmp_path / "countries.txt", tmp_path / "out.ics"
    assert main(["-f", str(lst), "add", "japan", "fr"]) == 0
    assert main(["-f", str(lst), "add", "narnia"]) == 1
    assert main(["-f", str(lst), "remove", "FR"]) == 0
    assert store.load(lst) == ["JP"]
    assert main(["-f", str(lst), "generate", "-y", "2027-2028", "-o", str(out)]) == 0
    text = out.read_text(encoding="utf-8")
    assert "DTSTART;VALUE=DATE:20270101" in text and "DTSTART;VALUE=DATE:20280101" in text


def test_generate_without_countries_fails(tmp_path):
    assert main(["-f", str(tmp_path / "none.txt"), "generate"]) == 1

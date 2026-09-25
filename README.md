# loci

Keep a list of countries, look up their public holidays, and export them as an
`.ics` calendar file you can import into Google Calendar, Outlook or Apple Calendar.

Holiday data comes from the [`holidays`](https://github.com/vacanza/holidays)
library, which covers 200+ countries and their states/provinces, and works offline.

## Install

```sh
pip install -e .
```

## Usage

```sh
# Build your list. Use ISO codes or English names; add -SUBDIVISION for regional holidays.
loci add japan DE US-CA GBR-SCT india
loci list
loci remove india

# Find a code
loci countries united

# Look before exporting (defaults to this year and next)
loci preview -y 2027

# Write the calendar file
loci generate                          # -> holidays.ics, this year + next
loci generate -y 2026-2028 -o team-holidays.ics --name "Team Holidays"
```

The list is saved in `countries.txt` in the current folder (one entry per line,
`#` for comments), so you can edit it by hand or commit it. Use `-f PATH` to
point at a different list.

Each event is an all-day entry titled like `🇯🇵 Japan: New Year's Day`, marked as
"free" so it doesn't block your schedule. Pass `--no-flags` to drop the emoji.

## Updating your calendar

Event IDs are stable, so re-importing a regenerated file updates existing events
rather than duplicating them in apps that honour UIDs (Apple Calendar, Outlook).
For Google Calendar, the cleanest approach is to create a dedicated
"International Holidays" calendar and import into that one:

- **Google Calendar**: Settings → Import & export → Import → choose the file and the target calendar.
- **Outlook**: File → Open & Export → Import/Export → *Import an iCalendar (.ics)*.
- **Apple Calendar**: File → Import, or double-click the file.

## Development

```sh
pip install -e '.[test]'
pytest
```

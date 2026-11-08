# plz-orte.tsv.gz

The German postal codes with their places, boroughs and suburbs that
Homes checks a PLZ and Ort against (`adresse.go`, `OrtFehler`).

- Source: the OpenPLZ API data, `src/de/osm/streets.csv` of
  <https://github.com/openpotato/openplzapi.data> (commit 9db4fa4,
  2025-11-22), itself extracted from OpenStreetMap.
- Licence: Open Database License (ODbL) 1.0,
  <https://opendatacommons.org/licenses/odbl/1-0/>.
  © OpenStreetMap contributors, <https://www.openstreetmap.org/copyright>.
- This file is a derived database under the same licence: one line per
  postal code, its places (Locality) joined by `;`, then its boroughs
  and suburbs, tab-separated, gzipped. `tools/plz/plz.py` makes it from
  the source file; nothing else was changed.

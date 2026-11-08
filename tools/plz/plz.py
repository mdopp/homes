"""plz-orte.tsv.gz for Homes' address check (internal/kontaktantwort).

Reads the OpenPLZ API data's German streets (src/de/osm/streets.csv of
github.com/openpotato/openplzapi.data, ODbL, (c) OpenStreetMap
contributors) and writes one line per postal code: the code, its places
(Locality) joined by ";", then its boroughs and suburbs, tab-separated,
gzipped without a time stamp so the same input gives the same bytes.

    python3 -I tools/plz/plz.py streets.csv internal/kontaktantwort/plz-orte.tsv.gz
"""
import collections
import csv
import gzip
import sys

src, out = sys.argv[1], sys.argv[2]
orte = collections.defaultdict(set)
teile = collections.defaultdict(set)


def name(v):
    v = ' '.join(v.split())
    return v if any(c.isalpha() for c in v) else ''


with open(src, newline='', encoding='utf-8') as f:
    for row in csv.DictReader(f):
        p = row['PostalCode'].strip()
        if len(p) != 5 or not p.isdigit():
            continue
        if name(row['Locality']):
            orte[p].add(name(row['Locality']))
        for k in ('Borough', 'Suburb'):
            if name(row[k]):
                teile[p].add(name(row[k]))
lines = []
for p in sorted(set(orte) | set(teile)):
    lines.append('\t'.join([p, ';'.join(sorted(orte[p]))] + sorted(teile[p] - orte[p])))
data = ('\n'.join(lines) + '\n').encode()
with open(out, 'wb') as o, gzip.GzipFile(fileobj=o, mode='wb', mtime=0, compresslevel=9, filename='') as g:
    g.write(data)
print(len(lines), 'codes,', len(data), 'bytes before gzip', file=sys.stderr)

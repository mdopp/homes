package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/ansicht"
	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/caldav/davfake"
	"github.com/mdopp/homes/internal/papierkorb"
	"github.com/mdopp/homes/internal/usage"
	"github.com/mdopp/homes/internal/usage/usagefake"
)

// owner 2026-11-28 („Alles alte kann weg“): what was stored under Haus'
// names is rewritten once at start — the calendar views and a deleted
// calendar under /haus/…, the PRODID on Homes' cards, the counts under the
// app haus — nothing is lost, and a second start changes nothing.
func TestUmbenennenOnce(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ansicht.File), []byte(`{"anna":{"aus":["/haus/abo-ferien/"],"farben":{"/haus/haushalt/":"#2f6fdf"}}}`), 0o600)
	korb := &papierkorb.Korb{Dir: dir}
	e, err := korb.Rein("kalender", "Urlaub", "anna", map[string]any{"path": "/haus/familie-urlaub/", "name": "Urlaub", "abzug": map[string]any{"objekte": []any{}}})
	if err != nil {
		t.Fatal(err)
	}
	liste, _ := korb.Rein("liste", "Baumarkt", "anna", map[string]any{"path": "/haus/x/"})

	dav := davfake.New("homes", "pw")
	t.Cleanup(dav.Close)
	dav.AddAddressBook("/homes/haushalt-kontakte/", "Haushalt")
	alt := "BEGIN:VCARD\r\nVERSION:4.0\r\nPRODID:-//mdopp//Haus Haushalt//DE\r\nUID:sw\r\nFN:Stadtwerke\r\nCATEGORIES:Firma\r\nEND:VCARD\r\n"
	dav.Put("/homes/haushalt-kontakte/sw.vcf", alt)
	family := "BEGIN:VCARD\r\nVERSION:3.0\r\nPRODID:-//Apple Inc.//iPhone OS 18.0//EN\r\nUID:paul\r\nFN:Paul\r\nEND:VCARD\r\n"
	dav.Put("/homes/haushalt-kontakte/paul.vcf", family)
	karten := &caldav.Client{URL: dav.URL, User: "homes", Password: "pw"}

	um := usagefake.New("tok")
	t.Cleanup(um.Close)
	um.Fenster = func(days string, counts map[usagefake.Count]int64) any { // usage-metrics: the window of the days up to today
		n, _ := strconv.Atoi(days)
		from := time.Date(2026, 11, 28-n+1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
		var es []map[string]any
		for k, c := range counts {
			if k.App == "haus" && k.Day >= from {
				es = append(es, map[string]any{"event": k.Event, "count": c})
			}
		}
		return map[string]any{"days": n, "from": from, "apps": []any{map[string]any{"app": "haus", "events": es}}}
	}
	zaehler := usage.New(um.URL, "tok")
	zaehler.Now = func() time.Time { return time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC) }
	zaehler.CountIn("haus", "seite.start")
	zaehler.Flush(context.Background())

	u := umbenennen{dir: dir, konto: "homes", views: ansicht.Open(dir), korb: korb, karten: karten, zaehler: zaehler}
	for run := 0; run < 2; run++ {
		if err := u.lokal(); err != nil {
			t.Fatal(err)
		}
		if err := u.entfernt(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ansicht.File)); strings.Contains(string(b), "/haus/") || !strings.Contains(string(b), `"/homes/haushalt/": "#2f6fdf"`) {
		t.Fatalf("ansicht: %s", b)
	}
	got, err := korb.Holen("kalender", e.ID)
	var d struct{ Path, Name string }
	json.Unmarshal(got.Daten, &d)
	if err != nil || d.Path != "/homes/familie-urlaub/" || d.Name != "Urlaub" || !got.Am.Equal(e.Am) {
		t.Fatalf("papierkorb: %v %s", err, got.Daten)
	}
	if l, _ := korb.Holen("liste", liste.ID); !strings.Contains(string(l.Daten), "/haus/x/") {
		t.Fatalf("another kind changed: %s", l.Daten)
	}
	cards, _, err := karten.Cards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, cd := range cards {
		if cd.UID == "sw" && (!cd.AsWritten() || strings.Replace(cd.Text(), "Homes Haushalt", "Haus Haushalt", 1) != alt) {
			t.Fatalf("Homes' card: %q", cd.Text())
		}
		if cd.UID == "paul" && cd.Text() != family {
			t.Fatalf("the family's card changed: %q", cd.Text())
		}
	}
	counts := um.Counts()
	if counts[usagefake.Count{App: usage.App, Event: "seite.start", Day: "2026-11-28"}] != 1 {
		t.Fatalf("counts: %v", counts)
	}
	for _, step := range []string{"ansicht", "papierkorb", "prodid", "zaehlungen"} {
		if _, err := os.Stat(u.marker(step)); err != nil {
			t.Errorf("no marker for %s", step)
		}
	}
}

// Radicale's account still haus, or unset: no path changes.
func TestNeuerPfad(t *testing.T) {
	for _, c := range []struct{ konto, in, want string }{
		{"homes", "/haus/haushalt/", "/homes/haushalt/"}, {"homes", "/anna/privat-x/", "/anna/privat-x/"},
		{"homes", "/hausboot/x/", "/hausboot/x/"}, {"haus", "/haus/haushalt/", "/haus/haushalt/"}, {"", "/haus/haushalt/", "/haus/haushalt/"},
	} {
		if got := (umbenennen{konto: c.konto}).neuerPfad(c.in); got != c.want {
			t.Errorf("%q %q → %q, want %q", c.konto, c.in, got, c.want)
		}
	}
}

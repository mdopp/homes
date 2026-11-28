package zugang

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func store(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	return &Store{Path: filepath.Join(t.TempDir(), Datei), Now: func() time.Time { return now }}, &now
}

func TestPruefenTrimsAndNamesTheField(t *testing.T) {
	a, f := Pruefen("  Oma \n Hilde ", " oma@example.org ", " Ich bin\ndie Oma. ")
	if f != "" || a.Name != "Oma Hilde" || a.Kontakt != "oma@example.org" || a.Nachricht != "Ich bin\ndie Oma." {
		t.Fatalf("%q %+v", f, a)
	}
	for _, c := range []struct {
		name, kontakt, text string
		want                Fehler
	}{
		{"", "x", "", OhneName},
		{strings.Repeat("ä", MaxName+1), "x", "", OhneName},
		{"a", "   ", "", OhneKontakt},
		{"a", strings.Repeat("k", MaxKontakt+1), "", OhneKontakt},
		{"a", "k", strings.Repeat("n", MaxText+1), ZuLang},
		{strings.Repeat("ä", MaxName), strings.Repeat("k", MaxKontakt), strings.Repeat("n", MaxText), ""},
	} {
		if _, f := Pruefen(c.name, c.kontakt, c.text); f != c.want {
			t.Errorf("%q/%q/%d: %q, want %q", c.name, c.kontakt, len(c.text), f, c.want)
		}
	}
}

// One accepted request per address an hour, twenty a day for everyone; the
// file keeps a salted hash of the address, never the address, and a restart
// (a new Store on the same file) keeps the limits.
func TestAnnehmenThrottlesAndHashes(t *testing.T) {
	s, now := store(t)
	a, err := s.Annehmen(Anfrage{Name: "Oma", Kontakt: "0711 1"}, "203.0.113.7")
	if err != nil || a.ID == "" || !a.Zeit.Equal(*now) {
		t.Fatalf("%v %+v", err, a)
	}
	if _, err := s.Annehmen(Anfrage{Name: "Oma", Kontakt: "0711 1"}, "203.0.113.7"); err != ErrZuViele {
		t.Errorf("the same address at once: %v, want ErrZuViele", err)
	}
	if _, err := s.Annehmen(Anfrage{Name: "Opa", Kontakt: "0711 2"}, "203.0.113.8"); err != nil {
		t.Errorf("another address: %v", err)
	}
	*now = now.Add(ProAdresse + time.Second)
	if _, err := s.Annehmen(Anfrage{Name: "Oma", Kontakt: "0711 1"}, "203.0.113.7"); err != nil {
		t.Errorf("the same address an hour later: %v", err)
	}
	b, _ := os.ReadFile(s.Path)
	if strings.Contains(string(b), "203.0.113") {
		t.Errorf("the file carries an address:\n%s", b)
	}
	if !strings.Contains(string(b), `"salz"`) || !strings.Contains(string(b), `"adresse"`) {
		t.Errorf("no salt or hash:\n%s", b)
	}
	// a new Store on the same file: the limits survive a restart
	s2 := &Store{Path: s.Path, Now: s.Now}
	if _, err := s2.Annehmen(Anfrage{Name: "Oma", Kontakt: "0711 1"}, "203.0.113.7"); err != ErrZuViele {
		t.Errorf("after a restart the same address: %v, want ErrZuViele", err)
	}
	for i := 3; i < ProTag; i++ {
		*now = now.Add(time.Second)
		if _, err := s2.Annehmen(Anfrage{Name: "N", Kontakt: "k"}, "198.51.100."+strconv.Itoa(i)); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if _, err := s2.Annehmen(Anfrage{Name: "N", Kontakt: "k"}, "192.0.2.1"); err != ErrZuViele {
		t.Errorf("the 21st of the day: %v, want ErrZuViele", err)
	}
	*now = now.Add(24 * time.Hour)
	if _, err := s2.Annehmen(Anfrage{Name: "N", Kontakt: "k"}, "192.0.2.1"); err != nil {
		t.Errorf("a day later: %v", err)
	}
}

func TestOffenAndErledigen(t *testing.T) {
	s, _ := store(t)
	a, _ := s.Annehmen(Anfrage{Name: "Oma", Kontakt: "0711 1", Nachricht: "Hallo"}, "203.0.113.7")
	b, _ := s.Annehmen(Anfrage{Name: "Opa", Kontakt: "0711 2"}, "203.0.113.8")
	offen, err := s.Offen()
	if err != nil || len(offen) != 2 || offen[0].ID != a.ID || offen[1].ID != b.ID || offen[0].Nachricht != "Hallo" {
		t.Fatalf("%v %+v", err, offen)
	}
	if err := s.Erledigen(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Erledigen("niemand"); err != nil {
		t.Errorf("an unknown id: %v", err)
	}
	offen, _ = s.Offen()
	if len(offen) != 1 || offen[0].ID != b.ID {
		t.Errorf("%+v", offen)
	}
}

// A file that cannot be read is an error, never an empty list: a request
// must not be lost, and the throttle must not forget.
func TestBrokenFileIsAnError(t *testing.T) {
	s, _ := store(t)
	os.WriteFile(s.Path, []byte("{"), 0o600)
	if _, err := s.Offen(); err == nil {
		t.Error("Offen read a broken file as nothing")
	}
	if _, err := s.Annehmen(Anfrage{Name: "N", Kontakt: "k"}, "192.0.2.1"); err == nil || err == ErrZuViele {
		t.Errorf("Annehmen on a broken file: %v", err)
	}
}

func TestNormal(t *testing.T) {
	for in, want := range map[string]string{
		" Ave.Ye.Koha655@GMAIL.com ":          "aveyekoha655@gmail.com",
		"ave.ye.koha655+preis@googlemail.com": "aveyekoha655@gmail.com",
		"mailto:Oma.Hilde+box@Example.org":    "oma.hilde@example.org", // dots stay outside gmail
		"0711 / 44-55 66":                     "0711445566",
		"+49 (711) 445566":                    "+49711445566",
		"  Ruf  mich AN ":                     "ruf mich an",
	} {
		if got := Normal(in); got != want {
			t.Errorf("Normal(%q) = %q, want %q", in, got, want)
		}
	}
}

// Ablehnen ticks off every open request from the sender, drops their words
// from the file and remembers the sender with when and by whom; the
// sender's next request is ErrAbgelehnt, leaves only the throttle's trace
// and counts against it; Zulassen forgets the sender.
func TestAblehnenAndZulassen(t *testing.T) {
	s, now := store(t)
	a, _ := s.Annehmen(Anfrage{Name: "Robertcox", Kontakt: "Ave.Ye.Koha655@gmail.com", Nachricht: "Xin chào"}, "203.0.113.7")
	s.Annehmen(Anfrage{Name: "Robert", Kontakt: "aveyekoha655+2@gmail.com"}, "203.0.113.8")
	oma, _ := s.Annehmen(Anfrage{Name: "Oma", Kontakt: "0711 1"}, "203.0.113.9")
	if k, err := s.Ablehnen("niemand", "anna"); k != "" || err != nil {
		t.Errorf("an unknown id: %q %v", k, err)
	}
	*now = now.Add(time.Minute)
	k, err := s.Ablehnen(a.ID, "clara")
	if err != nil || k != "aveyekoha655@gmail.com" {
		t.Fatalf("%q %v", k, err)
	}
	if offen, _ := s.Offen(); len(offen) != 1 || offen[0].ID != oma.ID {
		t.Errorf("open after Ablehnen: %+v", offen)
	}
	if g, err := s.Abgelehnte(); err != nil || len(g) != 1 || g[0] != (Sperre{Kontakt: k, Zeit: *now, Von: "clara"}) {
		t.Errorf("%v %+v", err, g)
	}
	b, _ := os.ReadFile(s.Path)
	if strings.Contains(string(b), "Robert") || strings.Contains(string(b), "Xin") {
		t.Errorf("the refused words stay:\n%s", b)
	}
	_, err = s.Annehmen(Anfrage{Name: "R", Kontakt: "AVEYEKOHA655@googlemail.com"}, "203.0.113.20")
	if !errors.Is(err, ErrAbgelehnt) || strings.Contains(err.Error(), "koha") {
		t.Fatalf("a refused sender: %v", err)
	}
	if offen, _ := s.Offen(); len(offen) != 1 {
		t.Errorf("a refused sender was kept: %+v", offen)
	}
	if _, err := s.Annehmen(Anfrage{Name: "R", Kontakt: "x@example.org"}, "203.0.113.20"); err != ErrZuViele {
		t.Errorf("the refused request's address at once: %v, want ErrZuViele", err)
	}
	if err := s.Zulassen(" AveYeKoha655@gmail.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.Zulassen("niemand@example.org"); err != nil {
		t.Errorf("one not refused: %v", err)
	}
	if g, _ := s.Abgelehnte(); len(g) != 0 {
		t.Errorf("still refused: %+v", g)
	}
	if _, err := s.Annehmen(Anfrage{Name: "R", Kontakt: "aveyekoha655@gmail.com"}, "203.0.113.21"); err != nil {
		t.Errorf("after Zulassen: %v", err)
	}
}

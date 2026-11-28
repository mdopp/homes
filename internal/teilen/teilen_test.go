package teilen

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// uhr is a clock the tests move.
type uhr struct{ t time.Time }

func (u *uhr) now() time.Time { return u.t }

func neuStore() (*Store, *uhr) {
	u := &uhr{time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC)}
	return &Store{Now: u.now}, u
}

// A session: a long random token, 30 minutes, its person's alone; what it shows wakes its page.
func TestSessionIsItsPersonsAndWakesThePage(t *testing.T) {
	s, u := neuStore()
	id, token, bis, err := s.Neu("michael", "Michael")
	if err != nil || len(token) != 43 || len(id) != 16 || !bis.Equal(u.t.Add(30*time.Minute)) {
		t.Fatalf("Neu: %q %q %v %v", id, token, bis, err)
	}
	a, wechsel, err := s.Zum(token, false)
	if err != nil || !a.Wartet() || a.Vorname != "Michael" || a.Geoeffnet {
		t.Fatalf("Zum: %+v %v", a, err)
	}
	if _, _, err := s.Zum(token[:42]+"x", false); !errors.Is(err, ErrWeg) {
		t.Errorf("another token: %v", err)
	}
	for _, err := range []error{s.Setzen(id, "anna", "", Freigabe{WLAN: &WLAN{SSID: "x"}}, Einladung{}), s.Beenden(id, "anna"), func() error { _, err := s.Stand(id, "anna"); return err }()} {
		if !errors.Is(err, ErrWeg) {
			t.Errorf("another person: %v", err)
		}
	}
	if err := s.Setzen(id, "michael", "", Freigabe{Karte: &Karte{Mobil: "0170 1"}}, Einladung{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-wechsel:
	default:
		t.Fatal("the page was not woken")
	}
	s.Zum(token, true)
	if st, _ := s.Stand(id, "michael"); !st.Geoeffnet || st.Karte.Mobil != "0170 1" {
		t.Errorf("Stand: %+v", st)
	}
}

// Ended or over: nothing released is left, the husk says how, and after Nachlauf it is gone.
func TestEndAndExpiryWipeWhatWasReleased(t *testing.T) {
	s, u := neuStore()
	id, token, _, _ := s.Neu("michael", "Michael")
	s.Setzen(id, "michael", "", Freigabe{WLAN: &WLAN{SSID: "Gast", Kennwort: "geheim"}}, Einladung{})
	if err := s.Beenden(id, "michael"); err != nil {
		t.Fatal(err)
	}
	a, _, _ := s.Zum(token, true)
	if !a.Beendet || a.WLAN != nil || a.Geoeffnet {
		t.Errorf("ended: %+v", a)
	}
	if err := s.Setzen(id, "michael", "", Freigabe{}, Einladung{}); !errors.Is(err, ErrVorbei) {
		t.Errorf("set after the end: %v", err)
	}
	id2, token2, _, _ := s.Neu("michael", "Michael")
	s.Setzen(id2, "michael", "", Freigabe{Karte: &Karte{Name: "Michael Dopp"}}, Einladung{})
	_, wechsel, _ := s.Zum(token2, false)
	u.t = u.t.Add(Dauer)
	a, _, _ = s.Zum(token2, false)
	if !a.Abgelaufen || a.Karte != nil {
		t.Errorf("over: %+v", a)
	}
	select {
	case <-wechsel:
	default:
		t.Error("the page was not woken at the end")
	}
	if _, err := s.Stand(id2, "michael"); !errors.Is(err, ErrVorbei) {
		t.Errorf("Stand over: %v", err)
	}
	u.t = u.t.Add(Nachlauf + time.Second)
	if _, _, err := s.Zum(token2, false); !errors.Is(err, ErrWeg) {
		t.Errorf("after Nachlauf: %v", err)
	}
}

// At most five open per person, at most twenty started an hour; another person is not counted.
func TestLimitsPerPerson(t *testing.T) {
	s, u := neuStore()
	var ids []string
	for range OffenJePerson {
		id, _, _, err := s.Neu("michael", "Michael")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if _, _, _, err := s.Neu("michael", "Michael"); !errors.Is(err, ErrZuViele) {
		t.Errorf("a sixth: %v", err)
	}
	if _, _, _, err := s.Neu("anna", "Anna"); err != nil {
		t.Errorf("anna: %v", err)
	}
	s.Beenden(ids[0], "michael")
	n := OffenJePerson
	for ; n < NeueJeStunde; n++ {
		id, _, _, err := s.Neu("michael", "Michael")
		if err != nil {
			t.Fatalf("start %d: %v", n, err)
		}
		s.Beenden(id, "michael")
	}
	if _, _, _, err := s.Neu("michael", "Michael"); !errors.Is(err, ErrZuOft) {
		t.Errorf("start %d within the hour: %v", n+1, err)
	}
	u.t = u.t.Add(time.Hour)
	if _, _, _, err := s.Neu("michael", "Michael"); err != nil {
		t.Errorf("an hour later: %v", err)
	}
}

func TestFieldsHeldToTheirShape(t *testing.T) {
	s, _ := neuStore()
	id, _, _, _ := s.Neu("michael", "Michael")
	for _, f := range []Freigabe{{Karte: &Karte{Geburtstag: "1.4.1990"}}, {WLAN: &WLAN{}}, {Karte: &Karte{Adresse: strings.Repeat("x", 301)}}} {
		if err := s.Setzen(id, "michael", "", f, Einladung{}); !errors.Is(err, ErrFeld) {
			t.Errorf("%+v: %v", f, err)
		}
	}
}

// The .vcf holds the released fields only, the name always (FN needs one), each value escaped.
func TestVCardOnlyReleasedFields(t *testing.T) {
	got := Karte{Mobil: "+49 170 1234567", Geburtstag: "1990-04-01"}.VCard("Michael")
	want := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Michael\r\nN:;Michael;;;\r\nTEL;TYPE=CELL:+49 170 1234567\r\nBDAY:1990-04-01\r\nEND:VCARD\r\n"
	if got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
	got = Karte{Name: "Michael Dopp", Mail: "m@example.org", Adresse: "Gartenstraße 7\n70173 Stuttgart", Firma: "Dopp, GmbH"}.VCard("Michael")
	for _, w := range []string{"FN:Michael Dopp\r\n", "N:Dopp;Michael;;;\r\n", "EMAIL;TYPE=INTERNET:m@example.org\r\n", `ADR:;;Gartenstraße 7\, 70173 Stuttgart;;;;` + "\r\n", `ORG:Dopp\, GmbH` + "\r\n"} {
		if !strings.Contains(got, w) {
			t.Errorf("no %q in\n%s", w, got)
		}
	}
	if strings.Contains(got, "TEL") || strings.Contains(got, "BDAY") {
		t.Errorf("a field not released:\n%s", got)
	}
}

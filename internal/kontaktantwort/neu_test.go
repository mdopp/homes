package kontaktantwort

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/caldav/davfake"
)

// neuAntwort is a new person's link of mdopp's, answered with nachher.
func neuAntwort(t *testing.T, s *Store, nachher Felder) Frage {
	t.Helper()
	link, _, err := s.Einladen("mdopp", "Michael", "https://kontakt.example.org")
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Beantworten(strings.TrimPrefix(link, "https://kontakt.example.org/"), Antwort{Aenderungen: Vergleich(Felder{}, nachher)})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// owner 2026-11-28: a new person's link is the review's link for an empty card, 7 days, once;
// several may be open at once.
func TestEinladen(t *testing.T) {
	now := time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC)
	s := &Store{Path: filepath.Join(t.TempDir(), Datei), Now: func() time.Time { return now }}
	link, f, err := s.Einladen("mdopp", "Michael", "https://kontakt.example.org/")
	if err != nil || !f.Neu || f.UID != "" || f.Buch != Haushalt || f.Von != "Michael" || !f.Bis.Equal(now.Add(7*24*time.Hour)) || f.Vorher.Name() != "" {
		t.Fatalf("%q %+v %v", link, f, err)
	}
	token := strings.TrimPrefix(link, "https://kontakt.example.org/")
	_, g, _ := s.Einladen("mdopp", "", "https://kontakt.example.org")
	if g.Von != "Michael" {
		t.Errorf("the app's link: von %q", g.Von)
	}
	if got, err := s.Zum(token); err != nil || got.ID != f.ID {
		t.Fatalf("the first link stopped working with the second: %v", err)
	}
	now = now.Add(7*24*time.Hour + time.Second)
	if _, err := s.Zum(token); err != ErrWeg {
		t.Errorf("after 7 days: %v", err)
	}
}

// What the answer holds, which household cards may be that person — the same mobile number, another
// same number, the same e-mail address, a similar name, the strongest first, five at most, each
// with its reasons — and the answer as changes of such a card.
func TestKandidatenUndErgaenzung(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), Datei)}
	f := neuAntwort(t, s, Felder{Vorname: "Erika", Nachname: "Muster", Telefone: []Telefon{{"0151 2345 6789", "mobil", false}, {"0711 445566", "privat", false}},
		Mails: []Mail{{"erika@example.org", "privat", false}}, Geburtstag: "--04-12"})
	if n := f.Eingetragen(); n.Name() != "Erika Muster" || len(n.Telefone) != 2 || n.Mails[0].Adresse != "erika@example.org" || n.Geburtstag != "--04-12" {
		t.Fatalf("Eingetragen: %+v", n)
	}
	all := []caldav.Contact{
		{UID: "b", Name: "Muster, Erika"},
		{UID: "c", Name: "Ben", Emails: []string{"ERIKA@example.org"}},
		{UID: "a", Name: "Erika Beispiel", Phones: []string{"+49 151 23456789"}},
		{UID: "d", Name: "Oma", Phones: []string{"+49 711 445566"}},
		{UID: "e", Name: "Erika Mustermann"},
		{Name: "Erika Muster", Phones: []string{"+49 151 23456789"}}, // no UID: nothing to write to
		{UID: "f", Given: "Erika", Family: "Müster", Name: "E. M.", Emails: []string{"erika@example.org"}},
	}
	got := Kandidaten(all, f, "49")
	var ks []string
	for _, k := range got {
		ks = append(ks, k.UID+":"+strings.Join(k.Gruende, "+"))
	}
	if s := strings.Join(ks, " "); s != "a:mobil f:mail+name c:mail d:telefon b:name" {
		t.Errorf("Kandidaten: %s", s)
	}
	for i := range 9 {
		all = append(all, caldav.Contact{UID: strconv.Itoa(i), Name: "Erika Muster"})
	}
	if got := Kandidaten(all, f, "49"); len(got) != KandidatenHoechstens || got[0].UID != "a" {
		t.Errorf("at most five, the strongest kept: %v", got)
	}
	if got := Kandidaten(all[4:5], f, "49"); got != nil {
		t.Errorf("another name: %v", got)
	}
	as := Ergaenzung(f, all[2])
	var arten []string
	for _, a := range as {
		arten = append(arten, a.Art+":"+a.Vorher+">"+a.Nachher)
	}
	if got := strings.Join(arten, " "); got != "nachname:Beispiel>Muster telefon:>0711 445566 mail:>erika@example.org geburtstag:>--04-12" {
		t.Errorf("the changes of Erika Beispiel: %s", got)
	}
}

// „Als neuen Kontakt anlegen“: a new card with the name always and what was ticked, nothing else;
// „… damit ergänzen“ writes the ticked into the matching card.
func TestNeuAnlegenUndErgaenzen(t *testing.T) {
	fake := davfake.New("homes", "pw")
	t.Cleanup(fake.Close)
	fake.AddAddressBook("/homes/haushalt-kontakte/", "Haushalt")
	fake.Put("/homes/haushalt-kontakte/a.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:a\r\nFN:Erika Beispiel\r\nN:Beispiel;Erika;;;\r\nTEL:+49 151 23456789\r\nEND:VCARD\r\n")
	dav := &caldav.Client{URL: fake.URL, User: "homes", Password: "pw"}
	s := &Store{Path: filepath.Join(t.TempDir(), Datei)}
	f := neuAntwort(t, s, Felder{Vorname: "Maria", Nachname: "von Bergen", Telefone: []Telefon{{"0151 2345 6789", "mobil", false}}, Mails: []Mail{{"maria@example.org", "privat", false}}})
	// vorname, nachname, telefon, mail: only the number ticked
	uid, stand, err := NeuAnlegen(context.Background(), dav, f, Stand(f, false, []int{2}), "49")
	if err != nil || uid == "" || strings.Join(stand, ",") != "uebernommen,uebernommen,uebernommen,abgelehnt" {
		t.Fatalf("%q %v %v", uid, stand, err)
	}
	card := ""
	for p, v := range fake.Objects() {
		if strings.Contains(p, strings.TrimPrefix(uid, "urn:uuid:")) {
			card = v
		}
	}
	for _, want := range []string{"FN:Maria von Bergen", "N:von Bergen;Maria;", "TEL", "151"} {
		if !strings.Contains(card, want) {
			t.Errorf("the new card lacks %q:\n%s", want, card)
		}
	}
	if strings.Contains(card, "maria@example.org") {
		t.Errorf("an unticked e-mail written:\n%s", card)
	}
	if _, err := s.EntscheidenNeu(f.ID, "mdopp", uid, Felder{}, f.Aenderungen, stand); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Eine(f.ID, "mdopp"); got.Name() != "Maria von Bergen" || got.UID != uid || got.ZuPruefen() || got.Aenderungen[0].Stand != Uebernommen {
		t.Errorf("kept: %+v", got)
	}
	if _, err := s.EntscheidenNeu(f.ID, "mdopp", uid, Felder{}, f.Aenderungen, stand); err != ErrWeg {
		t.Errorf("decided twice: %v", err)
	}

	g := neuAntwort(t, s, Felder{Vorname: "Erika", Nachname: "Beispiel", Telefone: []Telefon{{"0151 2345 6789", "mobil", false}}, Mails: []Mail{{"erika@example.org", "privat", false}}})
	ct := Kandidaten([]caldav.Contact{{UID: "a", Name: "Erika Beispiel", Given: "Erika", Family: "Beispiel", Phones: []string{"+49 151 23456789"}}}, g, "49")[0].Contact
	as, stand, err := Ergaenzen(context.Background(), dav, g, ct, []int{0}, "49")
	if err != nil || len(as) != 1 || as[0].Art != "mail" || stand[0] != Uebernommen {
		t.Fatalf("%+v %v %v", as, stand, err)
	}
	if card := fake.Objects()["/homes/haushalt-kontakte/a.vcf"]; !strings.Contains(card, "erika@example.org") || strings.Count(card, "TEL") != 1 {
		t.Errorf("Erika's card:\n%s", card)
	}
	if _, err := s.EntscheidenNeu(g.ID, "petra", "a", FelderVon(ct), as, stand); err != ErrWeg {
		t.Errorf("another person's review: %v", err)
	}
	if _, err := s.EntscheidenNeu(g.ID, "mdopp", "a", FelderVon(ct), as, stand); err != nil {
		t.Fatal(err)
	}
}

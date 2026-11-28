package kontaktantwort

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/texte"
)

// owner 2026-11-28: what a contact lacks of what the link asks for, in one order; a firm nothing.
func TestFehlt(t *testing.T) {
	for _, c := range []struct {
		name string
		ct   caldav.Contact
		want []string
	}{
		{"nothing at all", caldav.Contact{Name: "Oma Hilde"}, []string{FehltMobil, FehltMail, FehltAdresse, FehltGeburtstag}},
		{"a landline only", caldav.Contact{Name: "Oma Hilde", Phones: []string{"0711 445566"}, PhoneKinds: []string{"privat"}}, []string{FehltMobil, FehltMail, FehltAdresse, FehltGeburtstag}},
		{"a mobile by its digits", caldav.Contact{Name: "Ben", Phones: []string{"0171 1234567"}, Emails: []string{"b@example.org"}, Birthday: "--04-01"}, []string{FehltAdresse}},
		{"all there", caldav.Contact{Name: "Anna", Phones: []string{"+43 660 1234567"}, PhoneKinds: []string{"mobil"}, Emails: []string{"a@example.org"},
			Postal: []caldav.Address{{Street: "Hauptstraße 5", Locality: "Stadt"}}, Birthday: "1990-04-01"}, nil},
		{"Homes' firm", caldav.Contact{Name: "Stadtwerke", Org: "Stadtwerke", Categories: []string{caldav.FirmCategory}}, nil},
		{"a card named as its company", caldav.Contact{Name: "Bäckerei Müller", Org: "Bäckerei Müller"}, nil},
		{"no name", caldav.Contact{Phones: []string{"0171 1234567"}}, nil},
	} {
		if got := Fehlt(c.ct, ""); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	if got := FehltWorte(texte.Sprache("de"), []string{FehltMobil, FehltMail, FehltAdresse, FehltGeburtstag}); strings.Join(got, ",") != "Mobilnummer,E-Mail,Adresse,Geburtstag" {
		t.Errorf("the words: %v", got)
	}
}

// The mobile number for a WhatsApp or SMS link: the one marked mobil, else a German mobile by its digits.
func TestMobil(t *testing.T) {
	for want, ct := range map[string]caldav.Contact{
		"+4366012345678": {Phones: []string{"0711 1", "+43 660 12345678"}, PhoneKinds: []string{"privat", "mobil"}},
		"+4917112345678": {Phones: []string{"0711 445566", "0171 12345678"}},
		"+4915123456789": {Phones: []string{"+49 (0)151 23456789"}},
		"":               {Phones: []string{"0711 445566", "+43 1 234567"}},
	} {
		if got := Mobil(ct, "49"); got != want {
			t.Errorf("%v: %q, want %q", ct.Phones, got, want)
		}
	}
}

// The phone's contact found in the household's book: a number first, then an e-mail, then the
// name; several narrowed by the later ones; nothing found nil.
func TestFinden(t *testing.T) {
	all := []caldav.Contact{
		{UID: "a", Name: "Anna Beispiel", Phones: []string{"+49 151 23456789"}, Emails: []string{"anna@example.org"}},
		{UID: "b", Name: "Ben Beispiel", Phones: []string{"0711 445566"}, Emails: []string{"familie@example.org"}},
		{UID: "c", Name: "Clara Beispiel", Phones: []string{"0711 445566"}, Emails: []string{"clara@example.org"}},
		{UID: "d", Name: "Anna Beispiel", Emails: []string{"familie@example.org"}},
		{UID: "", Name: "Ohne UID", Phones: []string{"0170 1111111"}},
	}
	uids := func(cs []caldav.Contact) string {
		var out []string
		for _, ct := range cs {
			out = append(out, ct.UID)
		}
		return strings.Join(out, ",")
	}
	for _, c := range []struct {
		name, want  string
		heisst      string
		tels, mails []string
	}{
		{"the number however written", "a", "Irgendwer", []string{"0151-234 567 89"}, nil},
		{"the number before a wrong e-mail", "a", "", []string{"+4915123456789"}, []string{"clara@example.org"}},
		{"an e-mail, any case", "c", "", nil, []string{"Clara@Example.org"}},
		{"the name alone, accents aside", "a,d", "anna beispiel", nil, nil},
		{"two by number, narrowed by e-mail", "c", "", []string{"0711 445566"}, []string{"clara@example.org"}},
		{"two by number, narrowed by name", "b", "Ben Beispiel", []string{"0711 445566"}, nil},
		{"two by number, still two", "b,c", "", []string{"0711 445566"}, nil},
		{"two by e-mail, narrowed by name", "d", "Anna Beispiel", nil, []string{"familie@example.org"}},
		{"a card without a UID is none", "", "Ohne UID", []string{"0170 1111111"}, nil},
		{"nobody", "", "Dora", []string{"0170 2222222"}, []string{"dora@example.org"}},
	} {
		if got := uids(Finden(all, c.heisst, c.tels, c.mails, "49")); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// Link: the review link as „Link teilen“ makes it; the app has no first name, so the one last
// signed with is taken.
func TestLinkTakesTheLastFirstName(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), Datei)}
	ct := caldav.Contact{UID: "h1", Name: "Anna Beispiel", Given: "Anna", Family: "Beispiel"}
	link, f, err := s.Link("mdopp", "Michael", Haushalt, ct, "https://kontakt.example.org/")
	if err != nil || !strings.HasPrefix(link, "https://kontakt.example.org/") || f.Von != "Michael" || f.Vorher.Vorname != "Anna" || f.Buch != Haushalt {
		t.Fatalf("%q %+v %v", link, f, err)
	}
	if got, err := s.Zum(strings.TrimPrefix(link, "https://kontakt.example.org/")); err != nil || got.ID != f.ID {
		t.Fatalf("the token: %v", err)
	}
	if _, f, _ := s.Link("mdopp", "", Haushalt, caldav.Contact{UID: "h2", Name: "Ben"}, "https://kontakt.example.org"); f.Von != "Michael" {
		t.Errorf("from the app: von %q", f.Von)
	}
	if _, f, _ := s.Link("petra", "", Haushalt, ct, "https://kontakt.example.org"); f.Von != "" {
		t.Errorf("another person's name: %q", f.Von)
	}
}

package haushalt

import (
	"encoding/json"
	"strings"
	"testing"
)

type firmenListe struct {
	Kontakte  []firmKarte `json:"kontakte"`
	Teilweise bool        `json:"teilweise"`
}

func firmenLesen(t *testing.T, s setup) firmenListe {
	t.Helper()
	text, isErr := call(t, s.h, "firmenkontakte", map[string]any{})
	if isErr {
		t.Fatalf("firmenkontakte: %q", text)
	}
	var l firmenListe
	if err := json.Unmarshal([]byte(text[strings.LastIndex(text, "\n")+1:]), &l); err != nil {
		t.Fatalf("%q: %v", text, err)
	}
	return l
}

// Dokumente's kontakte_aufraeumen.py (owner 2026-11-09): the firm cards
// whole, a card a phone edited marked von_hand; a person's card not at all.
func TestFirmenkontakteListsFirmCards(t *testing.T) {
	s := newSetup(t, want{contacts: true})
	_, e := ergaenzen(t, s, map[string]any{"firma": "Union Krankenversicherung", "telefon": "0681 8440", "iban": ibanDE,
		"bank": "Beispielbank", "anschrift": map[string]any{"strasse": "Peter-Zimmer-Str. 2", "plz": "66123", "ort": "Saarbrücken"}})
	s.fake.Put("/bote/kontakte/hand.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nPRODID:-//Beispiel//Telefon//DE\r\nUID:hand\r\nFN:Union Krankenversicherung AG\r\n"+
		"ORG:Union Krankenversicherung AG\r\nCATEGORIES:Firma\r\nEND:VCARD\r\n")
	s.fake.Put("/bote/kontakte/paul.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:paul\r\nFN:Paul Beispiel\r\nTEL:0170 1234567\r\nEND:VCARD\r\n")
	l := firmenLesen(t, s)
	if len(l.Kontakte) != 2 || l.Teilweise {
		t.Fatalf("want the two firm cards: %+v", l)
	}
	by := map[string]firmKarte{}
	for _, k := range l.Kontakte {
		by[k.Kennung] = k
	}
	homes, hand := by[e.Kontakt], by["hand"]
	if homes.VonHand || homes.Name != "Union Krankenversicherung" || len(homes.Telefon) != 1 || len(homes.IBAN) != 1 || homes.IBAN[0] != "DE89370400440532013000" ||
		len(homes.Anschrift) != 1 || homes.Anschrift[0]["plz"] != "66123" || !strings.Contains(homes.Notiz, "Bank: Beispielbank") {
		t.Fatalf("Homes' card: %+v", homes)
	}
	if !hand.VonHand || hand.Name != "Union Krankenversicherung AG" {
		t.Fatalf("the phone's card: %+v", hand)
	}
}

// kontakt_entfernen removes only a firm card as Homes wrote it.
func TestKontaktEntfernenOnlyHomesOwnFirmCards(t *testing.T) {
	s := newSetup(t, want{contacts: true})
	_, e := ergaenzen(t, s, map[string]any{"firma": "Union Krankenversicherung Aktiengesellschaft"})
	_, amended := ergaenzen(t, s, map[string]any{"firma": "Union Krankenversicherung Aktiengesellschaft", "telefon": "0681 8440"})
	if amended.Status != "ergaenzt" {
		t.Fatalf("%+v", amended)
	}
	s.fake.Put("/bote/kontakte/hand.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nPRODID:-//Beispiel//Telefon//DE\r\nUID:hand\r\nFN:Union AG\r\nCATEGORIES:Firma\r\nEND:VCARD\r\n")
	s.fake.Put("/bote/kontakte/paul.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:paul\r\nFN:Paul Beispiel\r\nEND:VCARD\r\n")
	for _, c := range []struct{ kennung, want string }{
		{"hand", "von Hand bearbeitet"}, {"paul", "ohne Kategorie Firma"}, {"gibt-es-nicht", "kein Kontakt"}, {"", "kennung fehlt"},
	} {
		if text, isErr := call(t, s.h, "kontakt_entfernen", map[string]any{"kennung": c.kennung}); !isErr || !strings.Contains(text, c.want) {
			t.Fatalf("%q: %q (error %v), want %q", c.kennung, text, isErr, c.want)
		}
	}
	text, isErr := call(t, s.h, "kontakt_entfernen", map[string]any{"kennung": e.Kontakt})
	if isErr || !strings.Contains(text, "entfernt") || !strings.Contains(text, `"status":"entfernt"`) {
		t.Fatalf("%q %v", text, isErr)
	}
	if l := firmenLesen(t, s); len(l.Kontakte) != 1 || l.Kontakte[0].Kennung != "hand" {
		t.Fatalf("Homes' card amended once is still its own and gone; the phone's stays: %+v", l)
	}
}

// handy_firmen (owner 2026-11-09): the firms of the Immich key owner's own
// phone contacts, never a person, never another person's book, nothing
// while no owner is known.
func TestHandyFirmenOnlyTheKeyOwnersFirms(t *testing.T) {
	for _, wessen := range []string{"", "anna"} {
		s := newSetup(t, want{contacts: true, wessen: wessen})
		for _, p := range []string{"anna", "berta"} {
			s.fake.AddAddressBook("/"+p+"/handy-kontakte/", "Handy-Kontakte")
		}
		s.fake.Put("/anna/handy-kontakte/w.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:w\r\nFN:Wienkämper\r\nORG:Wienkämper\r\n"+
			"TEL:0541 123456\r\nADR;TYPE=WORK:;;Hafenstraße 12;Osnabrück;;49090;DE\r\nNOTE:privat\r\nEND:VCARD\r\n")
		s.fake.Put("/anna/handy-kontakte/o.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:o\r\nFN:Otto Muster\r\nN:Muster;Otto;;;\r\n"+
			"ORG:Wienkämper\r\nTEL:0170 1111111\r\nEND:VCARD\r\n")
		s.fake.Put("/berta/handy-kontakte/b.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:b\r\nFN:Bäckerei Korn\r\nORG:Bäckerei Korn\r\nEND:VCARD\r\n")
		text, isErr := call(t, s.h, "handy_firmen", map[string]any{})
		var l firmenListe
		if isErr || json.Unmarshal([]byte(text[strings.LastIndex(text, "\n")+1:]), &l) != nil {
			t.Fatalf("handy_firmen: %q", text)
		}
		if wessen == "" {
			if len(l.Kontakte) != 0 {
				t.Fatalf("no owner known: nothing, got %+v", l)
			}
			continue
		}
		if len(l.Kontakte) != 1 {
			t.Fatalf("want Anna's one firm, not Otto at the firm nor Berta's book: %+v", l)
		}
		k := l.Kontakte[0]
		if k.Name != "Wienkämper" || len(k.Telefon) != 1 || k.Telefon[0] != "0541 123456" || len(k.Anschrift) != 1 || k.Anschrift[0]["plz"] != "49090" || k.Notiz != "" {
			t.Fatalf("the firm whole, without the owner's note: %+v", k)
		}
	}
}

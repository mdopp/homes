package caldav

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

const korrekturCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:k1\r\nFN:Anna Beispiel\r\nN:Beispiel;Anna;;;\r\n" +
	"item1.TEL;TYPE=CELL:+49 151 2345 6789\r\nTEL;TYPE=HOME:0711 123\r\nEMAIL;TYPE=INTERNET:anna@example.org\r\n" +
	"ADR;TYPE=HOME:;;Weg 1;Stadt;BW;12345;DE\r\nORG:Firma A;Einkauf\r\nBDAY:1980-01-02\r\nX-IBAN:DE00\r\nNOTE:bleibt\r\nREV:20200101T000000Z\r\nEND:VCARD\r\n"

// A relation (owner 2026-11-09, netz) is added with its TYPE, the other by
// its UID or as a name in text; the same other twice is written once, and
// one is never changed or removed.
func TestCorrectedAddsARelationOnce(t *testing.T) {
	ks := []Korrektur{{Prop: "RELATED", Neu: "urn:uuid:h2", Art: "child"}, {Prop: "RELATED", Neu: "Oma Hilde", Art: "kin"},
		{Prop: "RELATED", Neu: "urn:uuid:h2", Art: "child"}, {Prop: "RELATED", Alt: "urn:uuid:h2", Neu: "urn:uuid:h3"}}
	got, done := Corrected(korrekturCard, "k1", ks, "20261109T100000Z")
	if want := []bool{true, true, true, false}; !slices.Equal(done, want) {
		t.Errorf("done %v, want %v", done, want)
	}
	if strings.Count(got, "RELATED") != 2 || !strings.Contains(got, "RELATED;TYPE=child:urn:uuid:h2\r\n") || !strings.Contains(got, "RELATED;TYPE=kin;VALUE=text:Oma Hilde\r\n") {
		t.Errorf("the relations:\n%s", got)
	}
}

func TestCorrectedChangesOnlyTheLinesOfTheTakenFields(t *testing.T) {
	ks := []Korrektur{
		{Prop: "N-FAMILY", Alt: "Beispiel", Neu: "Muster"},
		{Prop: "TEL", Alt: "+49 151 23456789", Neu: "+49 151 99999999", Art: "geschaeftlich"}, // the stored one written with spaces
		{Prop: "TEL", Alt: "0711 123", Neu: ""},
		{Prop: "EMAIL", Alt: "", Neu: "anna@muster.example"},
		{Prop: "ADR", AltAdr: &Address{Street: "Weg 1", Locality: "Stadt", Code: "12345", Country: "DE"}, NeuAdr: &Address{Street: "Weg 2", Locality: "Stadt", Code: "12345", Country: "DE"}},
		{Prop: "BDAY", Alt: "1980-01-02", Neu: "1980-01-03"},
		{Prop: "TEL", Alt: "0000 gone", Neu: "0171 1"}, // changed meanwhile: left alone
		{Prop: "ORG", Alt: "Firma A", Neu: "Firma B"},
		{Prop: "TITLE", Alt: "", Neu: "Leiterin"},
		{Prop: "TEL", Alt: "", Neu: "0711 5", Art: "privat"},
	}
	got, done := Corrected(korrekturCard, "k1", ks, "20261107T100000Z")
	if want := []bool{true, true, true, true, true, true, false, true, true, true}; !slices.Equal(done, want) {
		t.Errorf("done %v, want %v", done, want)
	}
	for _, want := range []string{"FN:Anna Muster\r\n", "N:Muster;Anna;;;\r\n", "item1.TEL;TYPE=WORK:+49 151 99999999\r\n", "ORG:Firma B;Einkauf\r\n", "TITLE:Leiterin\r\n", "TEL;TYPE=HOME:0711 5\r\n", "EMAIL;TYPE=INTERNET:anna@example.org\r\n",
		"EMAIL:anna@muster.example\r\n", "ADR;TYPE=HOME:;;Weg 2;Stadt;;12345;DE\r\n", "BDAY:19800103\r\n", "X-IBAN:DE00\r\n", "NOTE:bleibt\r\n", "REV:20261107T100000Z\r\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	for _, gone := range []string{"0711 123", "Weg 1", "1980-01-02", "20200101T000000Z", "0171 1"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q still in\n%s", gone, got)
		}
	}
	if _, done := Corrected(korrekturCard, "other", ks, "x"); slices.Contains(done, true) {
		t.Error("another card's UID was written")
	}
	if same, done := Corrected(korrekturCard, "k1", ks[6:7], "x"); done[0] || same != korrekturCard {
		t.Errorf("nothing taken, yet the card changed:\n%s", same)
	}
}

func TestCorrectSharedWritesOverTheVersionItRead(t *testing.T) {
	fake, c := schreibFake(t)
	ctx := context.Background()
	done, err := c.CorrectShared(ctx, "oma", []Korrektur{{Prop: "TEL", Alt: "+49 170 1", Neu: "+49 170 2"}, {Prop: "N-GIVEN", Alt: "Opa", Neu: "X"}, {Prop: "N-GIVEN", Alt: "Oma", Neu: "Omi"}})
	if err != nil || !slices.Equal(done, []bool{true, false, true}) {
		t.Fatalf("CorrectShared = %v, %v", done, err)
	}
	card := fake.Objects()["/homes/haushalt-kontakte/oma.vcf"]
	if !strings.Contains(card, "TEL;TYPE=cell:+49 170 2") || !strings.Contains(card, "FN:Omi Erna") || !strings.Contains(card, "N:Erna;Omi;;;") || !strings.Contains(card, "X-IBAN:DE00") || !strings.Contains(card, "PHOTO:") {
		t.Errorf("card after the write:\n%s", card)
	}
	if _, err := c.CorrectShared(ctx, "nobody", []Korrektur{{Prop: "TITLE", Alt: "", Neu: "b"}}); !errors.Is(err, ErrNoCard) {
		t.Errorf("a card that is not there: %v", err)
	}
}

// owner 2026-11-08: a number's or an address's kind and the main one,
// written the way the card's vCard version writes them (3.0 TYPE=PREF,
// 4.0 PREF=1), read back the same; the contact's own photo replaces the
// card's; the message goes below the note.
func TestCorrectedKindsTheMainOneThePhotoAndTheNote(t *testing.T) {
	ja, nein := true, false
	ks := []Korrektur{
		{Prop: "TEL", Alt: "0711 123", Neu: "0711 123", Haupt: &ja},                             // the second number becomes the main one
		{Prop: "EMAIL", Alt: "anna@example.org", Neu: "anna@example.org", Art: "geschaeftlich"}, // its kind, INTERNET kept
		{Prop: "TEL", Alt: "", Neu: "0151 1", Art: "andere"},
		{Prop: "PHOTO", Bild: []byte{0xff, 0xd8, 0xff}},
		{Prop: "NOTE", Neu: "Bin umgezogen."},
	}
	got, done := Corrected(strings.Replace(korrekturCard, "END:VCARD", "PHOTO;ENCODING=b;TYPE=JPEG:alt\r\nEND:VCARD", 1), "k1", ks, "x")
	if !slices.Equal(done, []bool{true, true, true, true, true}) {
		t.Errorf("done %v", done)
	}
	for _, want := range []string{"TEL;TYPE=HOME,PREF:0711 123\r\n", "item1.TEL;TYPE=CELL:+49 151 2345 6789\r\n", "EMAIL;TYPE=INTERNET,WORK:anna@example.org\r\n",
		"TEL:0151 1\r\n", "PHOTO;ENCODING=b;TYPE=JPEG:/9j/\r\n", "NOTE:bleibt\\n\\nBin umgezogen.\r\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("3.0: missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, ":alt") {
		t.Error("the card's old picture stays")
	}
	again, _ := Corrected(got, "k1", ks[4:], "x")
	if strings.Count(again, "Bin umgezogen") != 1 {
		t.Errorf("the message added twice:\n%s", again)
	}

	v4 := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:k4\r\nFN:A\r\nTEL;TYPE=cell;PREF=1:0171 1\r\nTEL;VALUE=uri;TYPE=home:tel:0711 2\r\nEND:VCARD\r\n"
	got, _ = Corrected(v4, "k4", []Korrektur{{Prop: "TEL", Alt: "0711 2", Neu: "0711 2", Haupt: &ja}, {Prop: "PHOTO", Bild: []byte{1}}}, "x")
	for _, want := range []string{"TEL;TYPE=CELL:0171 1\r\n", "TEL;VALUE=uri;TYPE=HOME;PREF=1:0711 2\r\n", "PHOTO:data:image/jpeg;base64,AQ==\r\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("4.0: missing %q in\n%s", want, got)
		}
	}
	got, _ = Corrected(v4, "k4", []Korrektur{{Prop: "TEL", Alt: "0171 1", Neu: "0171 1", Haupt: &nein}}, "x")
	if !strings.Contains(got, "TEL;TYPE=CELL:0171 1\r\n") {
		t.Errorf("the main one taken back:\n%s", got)
	}
	root, err := Parse(korrekturCard + v4)
	if err != nil {
		t.Fatal(err)
	}
	cs := Contacts(root)
	if !slices.Equal(cs[0].PhoneKinds, []string{"mobil", "privat"}) || !slices.Equal(cs[0].EmailKinds, []string{""}) ||
		!slices.Equal(cs[1].PhonePrefs, []bool{true, false}) || !slices.Equal(cs[1].PhoneKinds, []string{"mobil", "privat"}) {
		t.Errorf("read back: %+v %+v", cs[0], cs[1])
	}
}

package haushalt

import (
	"strings"
	"testing"
)

// The household's form (owner 2026-11-09): DIN 5008, international, the
// area code apart only where it is known for sure.
func TestTelefon(t *testing.T) {
	for in, want := range map[string]string{
		"08 00.4 10 01 516":  "+49 800 41001516", // the box's kind of mess, 2026-11-09
		"0 59 32.7 12 27":    "+49 5932 71227",
		"05401/90420":        "+49 5401 90420",
		"0180 2 100104":      "+49 1802 100104",
		"+49 (0)541 564890":  "+49 541 564890",
		"0049 541-564890":    "+49 541 564890",
		"tel:+4915123456789": "+49 151 23456789",
		"0151 23456789":      "+49 151 23456789",
		"0170/1234567":       "+49 170 1234567",
		"030 1234567":        "+49 30 1234567",
		"0906 12345":         "+49 906 12345",   // a three-digit area that is not …1
		"0340 123456":        "+49 340 123456",  // Dessau, three digits in 03…
		"0900 5123456":       "+49 900 5123456", // service
		"033201 12345":       "+49 3320112345",  // 03…: three to five digits, not split
		"0137 1234567":       "+49 1371234567",  // another 01… number: not split
		"+49 541 564890":     "+49 541 564890",  // already in form
		"+49 681 844-0":      "+49 681 844-0",   // an extension stays apart
		"030/310 07-26 47":   "+49 30 31007-2647",
		"05401 849395-0":     "+49 5401 849395-0",
		"0541-76099644":      "+49 541 76099644", // a hyphen after the area code is no extension
		"0179-2443258":       "+49 179 2443258",
	} {
		if got, ok := Telefon(in, "49"); !ok || got != want {
			t.Errorf("%q: %q %v, want %q", in, got, ok, want)
		}
	}
	// "08 00.4 10 01 15 16" (on the box): nine digits after 0800, one too many
	for _, in := range []string{"08 00.4 10 01 15 16", "0151 123456", "0541 123456 Durchwahl 12", "90420", "+43 660 1234567", "0541 1", "", "+49 541 5648901234567", "0541 12+34"} {
		if got, ok := Telefon(in, "49"); ok || got != in {
			t.Errorf("%q is unclear and stays: %q %v", in, got, ok)
		}
	}
	if got, ok := Telefon("0660 1234567", "43"); ok || got != "0660 1234567" { // a house abroad: no German table
		t.Errorf("Austria: %q %v", got, ok)
	}
}

// kontakt_anlegen writes the number in the household's form.
func TestKontaktAnlegenWritesTheForm(t *testing.T) {
	s := newSetup(t, want{contacts: true})
	if text, isErr := call(t, s.h, "kontakt_anlegen", map[string]any{"name": "Praxis Beispiel", "telefon": "05401/90420"}); isErr || !strings.Contains(text, "+49 5401 90420") {
		t.Fatalf("%q %v", text, isErr)
	}
	if _, vcf := onlyCard(t, s); !strings.Contains(vcf, "TEL:+49 5401 90420\r\n") {
		t.Fatalf("%s", vcf)
	}
}

// kontakt_ergaenzen compares a number by its digits: the same number in
// another form is there already, never a second line.
func TestKontaktErgaenzenSameNumberOtherForm(t *testing.T) {
	s := newSetup(t, want{contacts: true})
	s.fake.Put("/bote/kontakte/f.vcf", "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:f\r\nFN:Beispiel GmbH\r\nORG:Beispiel GmbH\r\nCATEGORIES:Firma\r\nTEL:05401/90420\r\nEND:VCARD\r\n")
	_, e := ergaenzen(t, s, map[string]any{"firma": "Beispiel GmbH", "telefon": "+49 5401 90420"})
	if e.Felder["telefon"] != "vorhanden" || e.Status != "unveraendert" {
		t.Fatalf("%+v", e)
	}
	if _, vcf := onlyCard(t, s); strings.Count(vcf, "TEL") != 1 {
		t.Fatalf("a second line:\n%s", vcf)
	}
}

// telefon_vereinheitlichen: the dry run lists and writes nothing; apply
// writes every household card's numbers in the form, a hand-edited card's
// too, each line's TYPE kept; an unclear number stays and is listed.
func TestTelefonVereinheitlichen(t *testing.T) {
	s := newSetup(t, want{contacts: true})
	s.fake.Put("/bote/kontakte/a.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nPRODID:-//Beispiel//Telefon//DE\r\nUID:a\r\nFN:Stadtwerke Beispiel\r\n"+
		"TEL;TYPE=WORK:08 00.4 10 01 516\r\nTEL:08 00.4 10 01 15 16\r\nTEL;TYPE=FAX:0180 2 100104\r\nTEL;TYPE=CELL:+49 151 23456789\r\nEND:VCARD\r\n")
	s.fake.Put("/bote/kontakte/b.vcf", "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:b\r\nFN:Praxis Beispiel\r\nTEL;TYPE=work:0 59 32.7 12 27\r\nTEL:90420\r\nEND:VCARD\r\n")
	before := s.fake.Objects()
	text, isErr := call(t, s.h, "telefon_vereinheitlichen", map[string]any{})
	if isErr {
		t.Fatalf("%q", text)
	}
	for _, want := range []string{"Probelauf: 3 Nummern", "Stadtwerke Beispiel: 08 00.4 10 01 516 → +49 800 41001516", "Stadtwerke Beispiel: 0180 2 100104 → +49 1802 100104",
		"Praxis Beispiel: 0 59 32.7 12 27 → +49 5932 71227", "Unklar:\nStadtwerke Beispiel: 08 00.4 10 01 15 16\nPraxis Beispiel: 90420"} {
		if !strings.Contains(text, want) {
			t.Fatalf("dry run lacks %q:\n%s", want, text)
		}
	}
	for p, v := range s.fake.Objects() {
		if before[p] != v {
			t.Fatalf("the dry run wrote %s", p)
		}
	}
	if text, isErr = call(t, s.h, "telefon_vereinheitlichen", map[string]any{"apply": true}); isErr || !strings.HasPrefix(text, "3 Nummern vereinheitlicht, 2 unklar") {
		t.Fatalf("%q", text)
	}
	objs := s.fake.Objects()
	for p, want := range map[string][]string{
		"/bote/kontakte/a.vcf": {"TEL;TYPE=WORK:+49 800 41001516\r\n", "TEL:08 00.4 10 01 15 16\r\n", "TEL;TYPE=FAX:+49 1802 100104\r\n", "TEL;TYPE=CELL:+49 151 23456789\r\n", "PRODID:-//Beispiel//Telefon//DE"},
		"/bote/kontakte/b.vcf": {"TEL;TYPE=work:+49 5932 71227\r\n", "TEL:90420\r\n"},
	} {
		for _, w := range want {
			if !strings.Contains(objs[p], w) {
				t.Fatalf("%s lacks %q:\n%s", p, w, objs[p])
			}
		}
	}
	if text, _ = call(t, s.h, "telefon_vereinheitlichen", map[string]any{}); !strings.HasPrefix(text, "Probelauf: 0 Nummern") {
		t.Fatalf("again: %q", text)
	}
}

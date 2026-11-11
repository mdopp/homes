package caldav

import (
	"context"
	"strings"
	"testing"
)

// One number however written: national, international, 00, a trunk (0).
func TestTelSchluessel(t *testing.T) {
	for _, same := range []string{"0171 1234567", "+49 171 1234567", "+49171-1234567", "0049 171 1234567", "+49 (0)171 1234567", "tel:+491711234567"} {
		if got := TelSchluessel(same, ""); got != "491711234567" {
			t.Errorf("TelSchluessel(%q) = %q", same, got)
		}
	}
	if TelSchluessel("+43 660 1234567", "") == TelSchluessel("0660 1234567", "") {
		t.Error("another country's number is not ours")
	}
	if TelSchluessel("0660 1234567", "43") != TelSchluessel("+43 660 1234567", "43") || TelSchluessel("", "") != "" {
		t.Error("the household's own country")
	}
}

// owner 2026-11-11: a correction that adds a number the card has in the
// other form (a browser's autofill +49 … to the card's 0171 …) adds no
// second line; red before (+491711234567 and 01711234567 were two).
func TestCorrectedAddsNoNumberTheCardHasInAnotherForm(t *testing.T) {
	card := strings.Replace(korrekturCard, "TEL;TYPE=HOME:0711 123\r\n", "TEL;TYPE=CELL:0171 1234567\r\n", 1)
	got, done := Corrected(card, "k1", []Korrektur{{Prop: "TEL", Neu: "+49 171 1234567"}}, "20261111T100000Z")
	if !done[0] || strings.Count(got, "TEL") != 2 || strings.Count(got, "171") != 1 {
		t.Fatalf("the number twice:\n%s", got)
	}
}

// A number changed into one the card has on another line already (the
// household's form for both of two lines) becomes one line: the one that
// says more stays, with the new value; red before (two equal lines).
func TestCorrectedChangesNoNumberIntoASecondCopy(t *testing.T) {
	card := strings.Replace(korrekturCard, "TEL;TYPE=HOME:0711 123\r\n", "TEL:0171 1234567\r\nTEL;TYPE=CELL,VOICE:+491711234567\r\n", 1)
	got, done := Corrected(card, "k1", []Korrektur{{Prop: "TEL", Alt: "0171 1234567", Neu: "+49 171 1234567"},
		{Prop: "TEL", Alt: "+491711234567", Neu: "+49 171 1234567"}}, "20261111T100000Z")
	if !done[0] || strings.Count(got, "171") != 1 || !strings.Contains(got, "TEL;TYPE=CELL,VOICE:+49 171 1234567\r\n") {
		t.Fatalf("done %v, the card:\n%s", done, got)
	}
}

// A card with one number several times keeps the line that says most (the
// main one, then the more kinds) where it is; every other line stays.
func TestOhneDoppelteNummern(t *testing.T) {
	card := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:a\r\nFN:A\r\nTEL;TYPE=cell:+49 171 1234567\r\nTEL;TYPE=CELL,VOICE,PREF:0171 1234567\r\n" +
		"item1.TEL:+49 171 1234567\r\nTEL;TYPE=home:0711 123\r\nTEL;TYPE=home:0711 123\r\nTEL:+43 711 123\r\nX-HAUS-INHALT:abc\r\nEND:VCARD\r\n"
	got, n := OhneDoppelteNummern(card, "")
	want := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:a\r\nFN:A\r\nTEL;TYPE=CELL,VOICE,PREF:0171 1234567\r\nTEL;TYPE=home:0711 123\r\nTEL:+43 711 123\r\nX-HAUS-INHALT:abc\r\nEND:VCARD\r\n"
	if n != 3 || got != want {
		t.Fatalf("%d removed:\n%s\nwant\n%s", n, got, want)
	}
	if got, n := OhneDoppelteNummern(want, ""); n != 0 || got != want {
		t.Fatalf("a card with each number once changes: %d\n%s", n, got)
	}
}

// The phone sends a number once per joined raw contact (Google, WhatsApp,
// Signal); the mirror stores it once, its hash still that of the text as
// sent, so the next sync of the same text writes nothing; red before.
func TestMirrorStoresANumberOnce(t *testing.T) {
	fake, c := schreibFake(t)
	ctx := context.Background()
	text := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:h1\r\nFN:Ben\r\nTEL;TYPE=cell,voice:+49 171 1234567\r\nTEL;TYPE=cell,voice:+49 171 1234567\r\nTEL;TYPE=other:+491711234567\r\nEND:VCARD\r\n"
	cards, err := SplitCards(text)
	if err != nil {
		t.Fatal(err)
	}
	m, err := c.Mirror(ctx, "/anna/handy-kontakte/", "Handy-Kontakte", cards)
	if err != nil || m.Neu != 1 {
		t.Fatalf("%+v %v", m, err)
	}
	stored := fake.Objects()["/anna/handy-kontakte/"+CardHref("h1")]
	if strings.Count(stored, "TEL") != 1 || !strings.Contains(stored, "TEL;TYPE=cell,voice:+49 171 1234567\r\n") {
		t.Fatalf("stored:\n%s", stored)
	}
	if m, err := c.Mirror(ctx, "/anna/handy-kontakte/", "Handy-Kontakte", cards); err != nil || m.Neu+m.Geaendert != 0 {
		t.Fatalf("the same text again wrote: %+v %v", m, err)
	}
}

package namen

import "testing"

func TestPlain(t *testing.T) {
	if got := Plain("Ärzte & Co."); got != "arzteundco" {
		t.Errorf("%q", got)
	}
}

// TestSichtbar: what changes how a name shows without showing goes; letters, accents, emoji and
// the newline stay.
func TestSichtbar(t *testing.T) {
	c := func(r rune) string { return string(r) }
	for in, want := range map[string]string{
		"Anna" + c(0x202e) + "egleB":                     "AnnaegleB",
		"An" + c(0x200b) + "na":                          "Anna",
		c(0xfeff) + "Anna" + c(0x2066) + "x" + c(0x2069): "Annax",
		"Ma" + c(0xad) + "rie":                           "Marie",
		"a" + c(0x2028) + "b" + c(0x2029) + "c":          "a b c",
		"x" + c(0x85) + "y" + c(0x9b) + "z":              "xyz",
		"Zoë 😀\nO'Brien\t":                               "Zoë 😀\nO'Brien\t",
	} {
		if got := Sichtbar(in); got != want {
			t.Errorf("Sichtbar(%q) = %q, want %q", in, got, want)
		}
	}
}

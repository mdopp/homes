// Package namen compares names as a person means them: a song, an artist,
// a film or a book written one way in Jellyfin, another in MusicBrainz or
// on the shopping list. It was namen.Plain; the music and the media
// packages share it, so it is no connector's.
package namen

import (
	"strings"
	"unicode"
)

// Plain is a name compared: lower case, letters and digits only, umlauts
// as their base letter.
func Plain(s string) string {
	r := strings.NewReplacer("ä", "a", "ö", "o", "ü", "u", "ß", "ss", "é", "e", "è", "e", "á", "a", "à", "a", "ó", "o", "í", "i", "&", "und")
	s = r.Replace(strings.ToLower(s))
	var b strings.Builder
	for _, c := range s {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// Sichtbar is s without the characters that change how text shows
// without showing themselves: bidi controls (U+202E turns „Anna“ round),
// zero-width and other format characters (Cf, the soft hyphen among
// them: „An<U+200B>na“ looks like „Anna“ and is not the same), C1
// controls; a line or paragraph separator becomes a space. A value a
// stranger typed (the public contact form, a shared card) is kept so,
// before an admin sees it or an address book stores it. The newline and
// the tab stay: the caller decides about them.
func Sichtbar(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == 0x2028 || r == 0x2029:
			return ' '
		case r >= 0x80 && r < 0xa0, unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, s)
}
